package xrayaws

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"unicode"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/google/go-cmp/cmp"
	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

func ignore(s string) string {
	var builder strings.Builder
	builder.Grow(len(s))
	for _, ch := range s {
		if unicode.IsLetter(ch) || unicode.IsNumber(ch) {
			builder.WriteRune('x')
		} else {
			builder.WriteRune(ch)
		}
	}
	return builder.String()
}

func ignoreVariableFieldFunc(in *schema.Segment) *schema.Segment {
	out := *in
	out.ID = ignore(out.ID)
	out.TraceID = ignore(out.TraceID)
	out.ParentID = ignore(out.ParentID)
	out.StartTime = 0
	out.EndTime = 0
	out.Subsegments = nil
	if out.AWS != nil {
		delete(out.AWS, "xray")
		if v, ok := out.AWS["request_id"].(string); ok {
			out.AWS["request_id"] = ignore(v)
		}
		if len(out.AWS) == 0 {
			out.AWS = nil
		}
	}
	if out.Cause != nil {
		for i := range out.Cause.Exceptions {
			out.Cause.Exceptions[i].ID = ignore(out.Cause.Exceptions[i].ID)
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
		Name:    "Test",
		ID:      "xxxxxxxxxxxxxxxx",
		TraceID: "x-xxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx",
		Subsegments: []*schema.Segment{
			{
				Name:      "lambda",
				ID:        "xxxxxxxxxxxxxxxx",
				Namespace: "aws",
				Subsegments: []*schema.Segment{
					{
						Name: "marshal",
						ID:   "xxxxxxxxxxxxxxxx",
					},
					{
						Name: "attempt",
						ID:   "xxxxxxxxxxxxxxxx",
						Subsegments: []*schema.Segment{
							{
								Name: "connect",
								ID:   "xxxxxxxxxxxxxxxx",
								Subsegments: []*schema.Segment{
									{
										Name: "dial",
										ID:   "xxxxxxxxxxxxxxxx",
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
							{
								Name: "request",
								ID:   "xxxxxxxxxxxxxxxx",
							},
						},
					},
					{
						Name: "unmarshal",
						ID:   "xxxxxxxxxxxxxxxx",
					},
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
					"request_id": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
					// "retries":    0.0,
				},
			},
		},
		Service: xray.ServiceData,
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestClient_FailDial(t *testing.T) {
	// setup dummy X-Ray daemon
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	var opt config.LoadOptions
	WithXRay()(&opt)
	cfg := aws.Config{
		Region: "fake-moon-1",
		EndpointResolver: aws.EndpointResolverFunc(func(service, region string) (aws.Endpoint, error) {
			return aws.Endpoint{
				// we expected no Gopher daemon on this computer ʕ◔ϖ◔ʔ
				URL:         "http://127.0.0.1:70",
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
	_, err := svc.ListFunctions(ctx, &lambda.ListFunctionsInput{})
	root.Close()
	if err == nil {
		t.Fatal("want error, but no error")
	}
	awsErr := err
	var urlErr *url.Error
	if !errors.As(awsErr, &urlErr) {
		t.Fatal(err)
	}

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
		Name:    "Test",
		ID:      "xxxxxxxxxxxxxxxx",
		TraceID: "x-xxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx",
		Subsegments: []*schema.Segment{
			{
				Name:      "lambda",
				ID:        "xxxxxxxxxxxxxxxx",
				Namespace: "aws",
				Fault:     true,
				Cause: &schema.Cause{
					WorkingDirectory: wd,
					Exceptions: []schema.Exception{
						{
							ID:      "xxxxxxxxxxxxxxxx",
							Message: awsErr.Error(),
							Type:    fmt.Sprintf("%T", awsErr),
						},
					},
				},
				Subsegments: []*schema.Segment{
					{
						Name: "marshal",
						ID:   "xxxxxxxxxxxxxxxx",
					},
					{
						Name:  "attempt",
						ID:    "xxxxxxxxxxxxxxxx",
						Fault: true,
						Subsegments: []*schema.Segment{
							{
								Name:  "connect",
								ID:    "xxxxxxxxxxxxxxxx",
								Fault: true,
								Subsegments: []*schema.Segment{
									{
										Name:  "dial",
										ID:    "xxxxxxxxxxxxxxxx",
										Fault: true,
										Cause: &schema.Cause{
											WorkingDirectory: wd,
											Exceptions: []schema.Exception{
												{
													ID:      "xxxxxxxxxxxxxxxx",
													Message: urlErr.Err.Error(),
													Type:    fmt.Sprintf("%T", urlErr.Err),
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
				HTTP: &schema.HTTP{
					Response: &schema.HTTPResponse{},
				},
				AWS: schema.AWS{
					"operation":  "ListFunctions",
					"region":     "fake-moon-1",
					"request_id": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
					// "retries":    0.0,
				},
			},
		},
		Service: xray.ServiceData,
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestClient_BadRequest(t *testing.T) {
	// setup dummy X-Ray daemon
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	// setup dummy aws service
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
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
	if err == nil {
		t.Fatal("want error, but no error")
	}
	awsErr := err
	var httpErr *awshttp.ResponseError
	if !errors.As(err, &httpErr) {
		t.Error("expected *smithyhttp.ResponseError, but not")
	}

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
		Name:    "Test",
		ID:      "xxxxxxxxxxxxxxxx",
		TraceID: "x-xxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx",
		Subsegments: []*schema.Segment{
			{
				Name:      "lambda",
				ID:        "xxxxxxxxxxxxxxxx",
				Namespace: "aws",
				Fault:     true,
				Cause: &schema.Cause{
					WorkingDirectory: wd,
					Exceptions: []schema.Exception{
						{
							ID:      "xxxxxxxxxxxxxxxx",
							Message: awsErr.Error(),
							Type:    fmt.Sprintf("%T", awsErr),
						},
					},
				},
				Subsegments: []*schema.Segment{
					{
						Name: "marshal",
						ID:   "xxxxxxxxxxxxxxxx",
					},
					{
						Name: "attempt",
						ID:   "xxxxxxxxxxxxxxxx",
						Subsegments: []*schema.Segment{
							{
								Name: "connect",
								ID:   "xxxxxxxxxxxxxxxx",
								Subsegments: []*schema.Segment{
									{
										Name: "dial",
										ID:   "xxxxxxxxxxxxxxxx",
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
							{
								Name: "request",
								ID:   "xxxxxxxxxxxxxxxx",
							},
						},
					},
					{
						Name:  "unmarshal",
						ID:    "xxxxxxxxxxxxxxxx",
						Fault: true,
						Cause: &schema.Cause{
							WorkingDirectory: wd,
							Exceptions: []schema.Exception{
								{
									ID:      "xxxxxxxxxxxxxxxx",
									Message: httpErr.Error(),
									Type:    fmt.Sprintf("%T", httpErr),
								},
							},
						},
					},
				},
				HTTP: &schema.HTTP{
					Response: &schema.HTTPResponse{
						Status:        400,
						ContentLength: 2,
					},
				},
				AWS: schema.AWS{
					"operation":  "ListFunctions",
					"region":     "fake-moon-1",
					"request_id": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
					// "retries":    0.0,
				},
			},
		},
		Service: xray.ServiceData,
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}
