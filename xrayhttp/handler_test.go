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

// compile time checking to satisfy the interface
// https://golang.org/doc/effective_go.html#blank_implements
var _ http.ResponseWriter = (*responseTracer)(nil)

func TestHandler(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	h := Handler(FixedTracingNamer("test"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// check optional interface of http.ResponseWriter
		if _, ok := w.(http.Hijacker); ok {
			t.Error("want not implement http.Hijacker, but it does")
		}
		if _, ok := w.(http.Flusher); !ok {
			t.Error("want implement http.Flusher, but it doesn't")
		}
		if _, ok := w.(http.Pusher); ok {
			t.Error("want not implement http.Pusher, but it does")
		}

		w.Header().Set("Content-Type", "text/plain")
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

	res := rec.Result()
	if res.StatusCode != http.StatusOK {
		t.Errorf("want %d, got %d", http.StatusOK, res.StatusCode)
	}
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Errorf("want %s, got %s", "hello", string(data))
	}
	if res.Header.Get("Content-Type") != "text/plain" {
		t.Errorf("want %s, got %s", "text/plain", res.Header.Get("Content-Type"))
	}
}
