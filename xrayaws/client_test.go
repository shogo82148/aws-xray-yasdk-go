package xrayaws

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"unicode"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/google/go-cmp/cmp"
	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
	"github.com/shogo82148/aws-xray-yasdk-go/xrayaws/whitelist"
)

// we check the format of strings, ignore their values.
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

const timeFilled = 1234567890

// we check wheather time is set
func ignoreTime(t float64) float64 {
	if t == 0 {
		return 0
	}
	return timeFilled
}

func ignoreVariableFieldFunc(in *schema.Segment) *schema.Segment {
	out := *in
	out.ID = ignore(out.ID)
	out.TraceID = ignore(out.TraceID)
	out.ParentID = ignore(out.ParentID)
	out.StartTime = ignoreTime(out.StartTime)
	out.EndTime = ignoreTime(out.EndTime)
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
		Name:      "Test",
		ID:        "xxxxxxxxxxxxxxxx",
		TraceID:   "x-xxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx",
		StartTime: timeFilled,
		EndTime:   timeFilled,
		Subsegments: []*schema.Segment{
			{
				Name:      "lambda",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
				Namespace: "aws",
				Subsegments: []*schema.Segment{
					{
						Name:      "marshal",
						ID:        "xxxxxxxxxxxxxxxx",
						StartTime: timeFilled,
						EndTime:   timeFilled,
					},
					{
						Name:      "attempt",
						ID:        "xxxxxxxxxxxxxxxx",
						StartTime: timeFilled,
						EndTime:   timeFilled,
						Subsegments: []*schema.Segment{
							{
								Name:      "connect",
								ID:        "xxxxxxxxxxxxxxxx",
								StartTime: timeFilled,
								EndTime:   timeFilled,
								Subsegments: []*schema.Segment{
									{
										Name:      "dial",
										ID:        "xxxxxxxxxxxxxxxx",
										StartTime: timeFilled,
										EndTime:   timeFilled,
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
								Name:      "request",
								ID:        "xxxxxxxxxxxxxxxx",
								StartTime: timeFilled,
								EndTime:   timeFilled,
							},
						},
					},
					{
						Name:      "unmarshal",
						ID:        "xxxxxxxxxxxxxxxx",
						StartTime: timeFilled,
						EndTime:   timeFilled,
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
					"request_id": "", // TODO: fix me
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
		Name:      "Test",
		ID:        "xxxxxxxxxxxxxxxx",
		TraceID:   "x-xxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx",
		StartTime: timeFilled,
		EndTime:   timeFilled,
		Subsegments: []*schema.Segment{
			{
				Name:      "lambda",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
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
						Name:      "marshal",
						ID:        "xxxxxxxxxxxxxxxx",
						StartTime: timeFilled,
						EndTime:   timeFilled,
					},
					{
						Name:      "attempt",
						ID:        "xxxxxxxxxxxxxxxx",
						StartTime: timeFilled,
						EndTime:   timeFilled,
						Fault:     true,
						Subsegments: []*schema.Segment{
							{
								Name:      "connect",
								ID:        "xxxxxxxxxxxxxxxx",
								StartTime: timeFilled,
								EndTime:   timeFilled,
								Fault:     true,
								Subsegments: []*schema.Segment{
									{
										Name:      "dial",
										ID:        "xxxxxxxxxxxxxxxx",
										StartTime: timeFilled,
										EndTime:   timeFilled,
										Fault:     true,
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
					"request_id": "", // TODO: fix me
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
	cfg := &aws.Config{
		Region:      aws.String("fake-moon-1"),
		Credentials: credentials.NewStaticCredentials("akid", "secret", "noop"),
		Endpoint:    aws.String(ts.URL),
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
	awsErr := err.(awserr.Error)

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
		Name:      "Test",
		ID:        "xxxxxxxxxxxxxxxx",
		TraceID:   "x-xxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx",
		StartTime: timeFilled,
		EndTime:   timeFilled,
		Subsegments: []*schema.Segment{
			{
				Name:      "lambda",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
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
						Name:      "marshal",
						ID:        "xxxxxxxxxxxxxxxx",
						StartTime: timeFilled,
						EndTime:   timeFilled,
					},
					{
						Name:      "attempt",
						ID:        "xxxxxxxxxxxxxxxx",
						StartTime: timeFilled,
						EndTime:   timeFilled,
						Subsegments: []*schema.Segment{
							{
								Name:      "connect",
								ID:        "xxxxxxxxxxxxxxxx",
								StartTime: timeFilled,
								EndTime:   timeFilled,
								Subsegments: []*schema.Segment{
									{
										Name:      "dial",
										ID:        "xxxxxxxxxxxxxxxx",
										StartTime: timeFilled,
										EndTime:   timeFilled,
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
								Name:      "request",
								ID:        "xxxxxxxxxxxxxxxx",
								StartTime: timeFilled,
								EndTime:   timeFilled,
							},
						},
					},
					{
						Name:      "unmarshal",
						ID:        "xxxxxxxxxxxxxxxx",
						StartTime: timeFilled,
						EndTime:   timeFilled,
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

func TestGetValue(t *testing.T) {
	type Foo struct {
		_   struct{}
		Bar int
	}

	if got, ok := getValue(Foo{Bar: 123}, "Bar").(int); !ok || got != 123 {
		t.Errorf("want %d, got %d", 123, got)
	}
	if got, ok := getValue(&Foo{Bar: 123}, "Bar").(int); !ok || got != 123 {
		t.Errorf("want %d, got %d", 123, got)
	}
	if got := getValue(Foo{Bar: 123}, "FooBar"); got != nil {
		t.Errorf("want %v, got %v", nil, got)
	}
}

func TestInsertDescriptor_map(t *testing.T) {
	type Map struct {
		_   struct{}
		Foo map[string]string
	}
	v := Map{Foo: map[string]string{"foo": "bar", "hoge": "fuga"}}
	aws := schema.AWS{}
	insertDescriptor(&whitelist.Descriptor{
		Map:     true,
		GetKeys: true,
	}, aws, v, "Foo")
	got := aws.Get("Foo").([]interface{})
	if len(got) != 2 {
		t.Errorf("want 2, got %d", len(got))
	}
	a := got[0].(string)
	b := got[1].(string)
	if !(a == "foo" && b == "hoge") && !(a == "hoge" && b == "foo") {
		t.Errorf("want %v, got %v", []string{"foo", "bar"}, got)
	}
}

func TestInsertDescriptor_list(t *testing.T) {
	type Map struct {
		_   struct{}
		Foo []string
	}
	v := Map{Foo: []string{"foo", "bar", "hoge", "fuga"}}
	aws := schema.AWS{}
	insertDescriptor(&whitelist.Descriptor{
		List:     true,
		GetCount: true,
	}, aws, v, "Foo")
	got := aws.Get("Foo").(int)
	if got != 4 {
		t.Errorf("want 4, got %d", got)
	}
}

func TestInsertDescriptor_value(t *testing.T) {
	type Map struct {
		_   struct{}
		Foo string
	}
	v := Map{Foo: "bar"}
	aws := schema.AWS{}
	insertDescriptor(&whitelist.Descriptor{
		RenameTo: "fizz_bazz",
	}, aws, v, "Foo")
	got := aws.Get("fizz_bazz").(string)
	if got != "bar" {
		t.Errorf("want bar, got %s", got)
	}
}
