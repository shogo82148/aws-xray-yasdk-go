package xrayaws

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
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
	if out.AWS != nil {
		delete(out.AWS, "xray")
		if len(out.AWS) == 0 {
			out.AWS = nil
		}
	}
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
		w.Header().Set("Content-Type", "application/json")
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

	var opt config.LoadOptions
	WithXRay()(&opt)
	cfg := aws.Config{
		Region: "fake-moon-1",
		EndpointResolver: aws.EndpointResolverFunc(func(service, region string) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:         ts.URL,
				SigningName: "lambda",
			}, nil
		}),
		Retryer: func() aws.Retryer {
			return aws.NopRetryer{}
		},
		APIOptions: opt.APIOptions,
	}

	// start testing
	svc := lambda.NewFromConfig(cfg)
	ctx, root := xray.BeginSegment(ctx, "Test")
	_, err = svc.ListFunctions(ctx, &lambda.ListFunctionsInput{})
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
				HTTP: &schema.HTTP{
					Response: &schema.HTTPResponse{
						Status:        200,
						ContentLength: 2,
					},
				},
				AWS: schema.AWS{
					"operation":  "ListFunctions",
					"region":     "fake-moon-1",
					"request_id": "",
					"retries":    0.0,
				},
			},
		},
		Service: xray.ServiceData,
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}
