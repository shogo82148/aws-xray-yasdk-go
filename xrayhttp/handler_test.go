package xrayhttp

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
		Name:      "test",
		ID:        "xxxxxxxxxxxxxxxx",
		TraceID:   "x-xxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx",
		StartTime: timeFilled,
		EndTime:   timeFilled,
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
			{
				Name:      "response",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
			},
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
	data, err := io.ReadAll(res.Body)
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
		select {
		case <-r.Context().Done():
		default:
			t.Error("the context should be canceled, but not")
		}

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := w.Write([]byte("canceled")); err != nil {
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
		Name:      "test",
		ID:        "xxxxxxxxxxxxxxxx",
		TraceID:   "x-xxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx",
		StartTime: timeFilled,
		EndTime:   timeFilled,
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
			{
				Name:      "response",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
			},
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
	data, err := io.ReadAll(res.Body)
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

type dummyStringWriter struct {
	http.ResponseWriter
	called bool
}

func (rw *dummyStringWriter) WriteString(s string) (int, error) {
	rw.called = true
	return rw.ResponseWriter.Write([]byte(s))
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
	rw := &dummyStringWriter{
		ResponseWriter: rec,
	}
	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	req = req.WithContext(ctx)
	h.ServeHTTP(rw, req)

	got, err := td.Recv()
	if err != nil {
		t.Fatal(err)
	}
	want := &schema.Segment{
		Name:      "test",
		ID:        "xxxxxxxxxxxxxxxx",
		TraceID:   "x-xxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx",
		StartTime: timeFilled,
		EndTime:   timeFilled,
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
			{
				Name:      "response",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
			},
		},
		Service: xray.ServiceData,
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
	if !rw.called {
		t.Error("WriteString is not called")
	}

	res := rec.Result()
	if res.StatusCode != http.StatusOK {
		t.Errorf("want %d, got %d", http.StatusOK, res.StatusCode)
	}
	data, err := io.ReadAll(res.Body)
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

type dummyReaderFrom struct {
	http.ResponseWriter
	called bool
}

func (rw *dummyReaderFrom) ReadFrom(r io.Reader) (n int64, err error) {
	rw.called = true
	return io.Copy(rw.ResponseWriter, r)
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
	}))

	rec := httptest.NewRecorder()
	rw := &dummyReaderFrom{
		ResponseWriter: rec,
	}
	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	req = req.WithContext(ctx)
	h.ServeHTTP(rw, req)

	got, err := td.Recv()
	if err != nil {
		t.Fatal(err)
	}
	want := &schema.Segment{
		Name:      "test",
		ID:        "xxxxxxxxxxxxxxxx",
		TraceID:   "x-xxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx",
		StartTime: timeFilled,
		EndTime:   timeFilled,
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
			{
				Name:      "response",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
			},
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
	data, err := io.ReadAll(res.Body)
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
		Name:      "test",
		ID:        "xxxxxxxxxxxxxxxx",
		TraceID:   "x-xxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx",
		StartTime: timeFilled,
		EndTime:   timeFilled,
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
			{
				Name:      "response",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
			},
		},
		Service: xray.ServiceData,
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

type dummyFlusher struct {
	http.ResponseWriter
	called bool
}

func (rw *dummyFlusher) Flush() {
	rw.called = true
}

func TestHandler_Flush(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	h := Handler(FixedTracingNamer("test"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		if _, err := io.WriteString(w, "hello"); err != nil {
			panic(err)
		}
		w.(http.Flusher).Flush()
	}))

	rec := httptest.NewRecorder()
	rw := &dummyFlusher{
		ResponseWriter: rec,
	}
	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	req = req.WithContext(ctx)
	h.ServeHTTP(rw, req)

	got, err := td.Recv()
	if err != nil {
		t.Fatal(err)
	}
	want := &schema.Segment{
		Name:      "test",
		ID:        "xxxxxxxxxxxxxxxx",
		TraceID:   "x-xxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx",
		StartTime: timeFilled,
		EndTime:   timeFilled,
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
			{
				Name:      "response",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
			},
		},
		Service: xray.ServiceData,
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
	if !rw.called {
		t.Error("Flush is not called")
	}

	res := rec.Result()
	if res.StatusCode != http.StatusOK {
		t.Errorf("want %d, got %d", http.StatusOK, res.StatusCode)
	}
	data, err := io.ReadAll(res.Body)
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
		Name:      "test",
		ID:        "xxxxxxxxxxxxxxxx",
		TraceID:   "x-xxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx",
		StartTime: timeFilled,
		EndTime:   timeFilled,
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
				Name:      "response",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
				Fault:     true,
			},
		},
		Fault: true,
		Cause: &schema.Cause{
			WorkingDirectory: wd,
			Exceptions: []schema.Exception{
				{
					ID:      "xxxxxxxxxxxxxxxx",
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

func TestGetURL(t *testing.T) {
	tests := []struct {
		name string
		req  *http.Request
		want string
	}{
		{
			name: "simple",
			req: &http.Request{
				Host: "example.com",
				URL:  &url.URL{Path: "/"},
			},
			want: "http://example.com/",
		},
		{
			name: "https",
			req: &http.Request{
				Host: "example.com",
				URL:  &url.URL{Path: "/"},
				TLS:  &tls.ConnectionState{},
			},
			want: "https://example.com/",
		},
		{
			name: "x-forwarded-proto",
			req: &http.Request{
				Header: http.Header{"X-Forwarded-Proto": []string{"https"}},
				Host:   "example.com",
				URL:    &url.URL{Path: "/"},
			},
			want: "https://example.com/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getURL(tt.req)
			if got != tt.want {
				t.Errorf("want %s, got %s", tt.want, got)
			}
		})
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name      string
		req       *http.Request
		wantIP    string
		forwarded bool
	}{
		{
			name: "simple",
			req: &http.Request{
				RemoteAddr: "192.0.2.1:48011",
			},
			wantIP:    "192.0.2.1",
			forwarded: false,
		},
		{
			name: "ipv6",
			req: &http.Request{
				RemoteAddr: "[2001:db8::1]:48011",
			},
			wantIP:    "2001:db8::1",
			forwarded: false,
		},
		{
			name: "xff",
			req: &http.Request{
				Header: http.Header{
					"X-Forwarded-For": []string{"198.51.100.1"},
				},
				RemoteAddr: "192.0.2.1:48011",
			},
			wantIP:    "198.51.100.1",
			forwarded: true,
		},
		{
			name: "forwarded-header",
			req: &http.Request{
				Header: http.Header{
					"Forwarded": []string{"for=198.51.100.1"},
				},
				RemoteAddr: "192.0.2.1:48011",
			},
			wantIP:    "198.51.100.1",
			forwarded: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIP, forwarded := clientIP(tt.req)
			if gotIP != tt.wantIP {
				t.Errorf("want %s, got %s", tt.wantIP, gotIP)
			}
			if forwarded != tt.forwarded {
				t.Errorf("want %t, got %t", tt.forwarded, forwarded)
			}
		})
	}
}
