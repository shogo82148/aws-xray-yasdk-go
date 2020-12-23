package xrayhttp

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

// compile time checking to satisfy the interface
// https://golang.org/doc/effective_go.html#blank_implements
var _ http.ResponseWriter = (*serverResponseTracer)(nil)
var _ TracingNamer = FixedTracingNamer("")
var _ TracingNamer = DynamicTracingNamer{}

func TestFixedTracingNamer(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	namer := FixedTracingNamer("segment-name")
	name := namer.TracingName(req)
	if name != "segment-name" {
		t.Errorf("want %s, got %s", "segment-name", name)
	}
}

func TestDynamicTracingNamer(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("match", func(t *testing.T) {
		namer := DynamicTracingNamer{RecognizedHosts: "*"}
		name := namer.TracingName(req)
		if name != "example.com" {
			t.Errorf("want %s, got %s", "example.com", name)
		}
	})
	t.Run("fallback", func(t *testing.T) {
		namer := DynamicTracingNamer{RecognizedHosts: "some.invalid", FallbackName: "fallback"}
		name := namer.TracingName(req)
		if name != "fallback" {
			t.Errorf("want %s, got %s", "fallback", name)
		}
	})
}

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
		Subsegments: []*schema.Segment{
			{Name: "response"},
		},
		Service: xray.ServiceData,
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

func TestHandler_context_canceled(t *testing.T) {
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

		// return InternalServerError if context canceled
		select {
		case <-r.Context().Done():
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusInternalServerError)
			if _, err := w.Write([]byte("canceled")); err != nil {
				panic(err)
			}
			return
		default:
		}

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("hello")); err != nil {
			panic(err)
		}
	}))

	ctx, cancel := context.WithCancel(ctx)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	req = req.WithContext(ctx)

	cancel()
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
				Status:        http.StatusInternalServerError,
				ContentLength: 8,
			},
		},
		Subsegments: []*schema.Segment{
			{Name: "response"},
		},
		Service: xray.ServiceData,
		Error:   true, // should be marked as error, not fault.
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}

	res := rec.Result()
	if res.StatusCode != http.StatusInternalServerError {
		t.Errorf("want %d, got %d", http.StatusInternalServerError, res.StatusCode)
	}
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "canceled" {
		t.Errorf("want %s, got %s", "canceled", string(data))
	}
	if res.Header.Get("Content-Type") != "text/plain" {
		t.Errorf("want %s, got %s", "text/plain", res.Header.Get("Content-Type"))
	}
}

func TestHandler_WriteString(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	h := Handler(FixedTracingNamer("test"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		if _, err := io.WriteString(w, "hello"); err != nil {
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
		Subsegments: []*schema.Segment{
			{Name: "response"},
		},
		Service: xray.ServiceData,
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

func TestHandler_ReadFrom(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	h := Handler(FixedTracingNamer("test"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		src := strings.NewReader("hello")
		dst := w.(io.ReaderFrom)
		if _, err := dst.ReadFrom(src); err != nil {
			panic(err)
		}
		f := w.(http.Flusher)
		f.Flush()
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
		Subsegments: []*schema.Segment{
			{Name: "response"},
		},
		Service: xray.ServiceData,
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

func TestHandler_Hijack(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	client := xray.ContextClient(ctx)
	h := HandlerWithClient(FixedTracingNamer("test"), client, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "webserver doesn't support hijacking", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusSwitchingProtocols)
		conn, _, err := hj.Hijack()
		if err != nil {
			panic(err)
		}
		defer conn.Close()
	}))
	ts := httptest.NewServer(h)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/hijack", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	got, err := td.Recv()
	if err != nil {
		t.Fatal(err)
	}
	want := &schema.Segment{
		Name: "test",
		HTTP: &schema.HTTP{
			Request: &schema.HTTPRequest{
				Method:    http.MethodGet,
				URL:       ts.URL + "/hijack",
				ClientIP:  "127.0.0.1",
				UserAgent: "Go-http-client/1.1",
			},
			Response: &schema.HTTPResponse{
				Status: http.StatusSwitchingProtocols,
			},
		},
		Subsegments: []*schema.Segment{
			{Name: "response"},
		},
		Service: xray.ServiceData,
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestHandler_Panic(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	client := xray.ContextClient(ctx)
	h := HandlerWithClient(FixedTracingNamer("test"), client, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		panic(http.ErrAbortHandler)
	}))
	ts := httptest.NewServer(h)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/panic", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		resp.Body.Close()
		t.Fatal("want error, got nil")
	}

	got, err := td.Recv()
	if err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	want := &schema.Segment{
		Name: "test",
		HTTP: &schema.HTTP{
			Request: &schema.HTTPRequest{
				Method:    http.MethodGet,
				URL:       ts.URL + "/panic",
				ClientIP:  "127.0.0.1",
				UserAgent: "Go-http-client/1.1",
			},
		},
		Subsegments: []*schema.Segment{
			{
				Name:  "response",
				Fault: true,
			},
		},
		Fault: true,
		Cause: &schema.Cause{
			WorkingDirectory: wd,
			Exceptions: []schema.Exception{
				{
					Message: fmt.Sprintf("%T: %s", http.ErrAbortHandler, http.ErrAbortHandler.Error()),
					Type:    "*xray.errorPanic",
				},
			},
		},
		Service: xray.ServiceData,
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}
