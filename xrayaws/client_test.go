package xrayaws

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/google/go-cmp/cmp"
	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

func ignoreVariableFieldFunc(in *schema.Segment) *schema.Segment {
	out := *in
	out.ID = ""
	out.TraceID = ""
	out.ParentID = ""
	out.StartTime = 0
	out.EndTime = 0
	out.Subsegments = nil
	if out.Cause != nil {
		for i := range out.Cause.Exceptions {
			out.Cause.Exceptions[i].ID = ""
		}
	}
	for _, sub := range in.Subsegments {
		out.Subsegments = append(out.Subsegments, ignoreVariableFieldFunc(sub))
	}
	return &out
}

// some fields change every execution, ignore them.
var ignoreVariableField = cmp.Transformer("Segment", ignoreVariableFieldFunc)

func TestClient(t *testing.T) {
	// setup dummy X-Ray daemon
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	// setup dummy aws service
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := io.WriteString(w, "{}"); err != nil {
			panic(err)
		}
	}))
	defer ts.Close()
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &aws.Config{
		Region:      aws.String("fake-moon-1"),
		Credentials: credentials.NewStaticCredentials("akid", "secret", "noop"),
		Endpoint:    aws.String(ts.URL),
	}
	s, err := session.NewSession(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// start testing
	svc := lambda.New(s)
	Client(svc.Client)
	ctx, root := xray.BeginSegment(ctx, "Test")
	_, err = svc.ListFunctionsWithContext(ctx, &lambda.ListFunctionsInput{})
	root.Close()
	if err != nil {
		t.Fatal(err)
	}

	// check the segment
	got, err := td.Recv()
	if err != nil {
		t.Fatal(err)
	}
	want := &schema.Segment{
		Name: "Test",
		Subsegments: []*schema.Segment{
			{
				Name:      "lambda",
				Namespace: "aws",
				Subsegments: []*schema.Segment{
					{Name: "marshal"},
					{
						Name: "attempt",
						Subsegments: []*schema.Segment{
							{
								Name: "connect",
								Subsegments: []*schema.Segment{
									{
										Name: "dial",
										Metadata: map[string]interface{}{
											"http": map[string]interface{}{
												"dial": map[string]interface{}{
													"network": "tcp",
													"address": u.Host,
												},
											},
										},
									},
								},
							},
							{Name: "request"},
						},
					},
					{Name: "unmarshal"},
				},
			},
		},
		Service: xray.ServiceData,
		AWS: &schema.AWS{
			XRay: &schema.XRay{
				Version: xray.Version,
				Type:    xray.Type,
			},
		},
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestClient_FailDial(t *testing.T) {
	// setup dummy X-Ray daemon
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	cfg := &aws.Config{
		Region:      aws.String("fake-moon-1"),
		Credentials: credentials.NewStaticCredentials("akid", "secret", "noop"),
		// we expected no Gopher daemon on this computer ʕ◔ϖ◔ʔ
		Endpoint: aws.String("http://127.0.0.1:70"),
		// we know this request will fail, no need to retry.
		MaxRetries: aws.Int(0),
	}
	s, err := session.NewSession(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// start testing
	svc := lambda.New(s)
	Client(svc.Client)
	ctx, root := xray.BeginSegment(ctx, "Test")
	_, err = svc.ListFunctionsWithContext(ctx, &lambda.ListFunctionsInput{})
	root.Close()
	if err == nil {
		t.Fatal("want error, but no error")
	}
	awsErr := err.(awserr.Error)
	urlErr := awsErr.OrigErr().(*url.Error)

	// check the segment
	got, err := td.Recv()
	if err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	want := &schema.Segment{
		Name: "Test",
		Subsegments: []*schema.Segment{
			{
				Name:      "lambda",
				Namespace: "aws",
				Fault:     true,
				Cause: &schema.Cause{
					WorkingDirectory: wd,
					Exceptions: []schema.Exception{
						{
							Message: awsErr.Error(),
							Type:    "*awserr.baseError",
						},
					},
				},
				Subsegments: []*schema.Segment{
					{Name: "marshal"},
					{
						Name:  "attempt",
						Fault: true,
						Subsegments: []*schema.Segment{
							{
								Name:  "connect",
								Fault: true,
								Subsegments: []*schema.Segment{
									{
										Name:  "dial",
										Fault: true,
										Cause: &schema.Cause{
											WorkingDirectory: wd,
											Exceptions: []schema.Exception{
												{
													Message: urlErr.Err.Error(),
													Type:    "*net.OpError",
												},
											},
										},
										Metadata: map[string]interface{}{
											"http": map[string]interface{}{
												"dial": map[string]interface{}{
													"network": "tcp",
													"address": "127.0.0.1:70",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		Service: xray.ServiceData,
		AWS: &schema.AWS{
			XRay: &schema.XRay{
				Version: xray.Version,
				Type:    xray.Type,
			},
		},
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}
