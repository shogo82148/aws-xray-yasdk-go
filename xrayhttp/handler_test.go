package xrayhttp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

// compile time checking to satisfy the interface
// https://golang.org/doc/effective_go.html#blank_implements
var _ http.ResponseWriter = (*responseTracer)(nil)

func TestHandler(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	h := Handler(FixedTracingNamer("test"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("hello")); err != nil {
			panic(err)
		}
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	req = req.WithContext(ctx)
	h.ServeHTTP(rec, req)

	got, err := td.Recv()
	if err != nil {
		t.Fatal(err)
	}
	want := &schema.Segment{
		Name: "test",
		HTTP: &schema.HTTP{
			Request: &schema.HTTPRequest{
				Method:   http.MethodGet,
				URL:      "http://example.com",
				ClientIP: "192.0.2.1",
			},
			Response: &schema.HTTPResponse{
				Status:        http.StatusOK,
				ContentLength: 5,
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
