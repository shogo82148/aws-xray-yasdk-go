package xrayhttp

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

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
	for _, sub := range in.Subsegments {
		out.Subsegments = append(out.Subsegments, ignoreVariableFieldFunc(sub))
	}
	return &out
}

// some fields change every execution, ignore them.
var ignoreVariableField = cmp.Transformer("Segment", ignoreVariableFieldFunc)

func TestClient(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("hello")); err != nil {
			panic(err)
		}
	}))
	defer ts.Close()

	func() {
		client := Client(nil)
		ctx, root := xray.BeginSegment(ctx, "test")
		defer root.Close()
		req, err := http.NewRequest(http.MethodGet, ts.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		req = req.WithContext(ctx)
		req.Host = "example.com"
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal()
		}
		defer resp.Body.Close()
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatal()
		}
		if string(data) != "hello" {
			t.Errorf("want %q, got %q", "hello", string(data))
		}
	}()

	got, err := td.Recv()
	if err != nil {
		t.Fatal(err)
	}
	want := &schema.Segment{
		Name: "test",
		Subsegments: []*schema.Segment{
			{
				Name:      "example.com",
				Namespace: "remote",
				Subsegments: []*schema.Segment{
					{
						Name: "connect",
						Subsegments: []*schema.Segment{
							{Name: "dial"},
						},
					},
					{Name: "request"},
				},
			},
		},
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}