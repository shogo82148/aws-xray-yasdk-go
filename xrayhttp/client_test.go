package xrayhttp

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/google/go-cmp/cmp"
	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
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
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	ch := make(chan xray.TraceHeader, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceHeader := xray.ParseTraceHeader(r.Header.Get(xray.TraceIDHeaderKey))
		ch <- traceHeader
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("hello")); err != nil {
			panic(err)
		}
	}))
	defer ts.Close()
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}

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
		Name:      "test",
		ID:        "xxxxxxxxxxxxxxxx",
		TraceID:   "x-xxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx",
		StartTime: timeFilled,
		EndTime:   timeFilled,
		Subsegments: []*schema.Segment{
			{
				Name:      "example.com",
				Namespace: "remote",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
				HTTP: &schema.HTTP{
					Request: &schema.HTTPRequest{
						Method: http.MethodGet,
						URL:    ts.URL,
					},
					Response: &schema.HTTPResponse{
						Status:        http.StatusOK,
						ContentLength: 5,
					},
				},
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
					{
						Name:      "response",
						ID:        "xxxxxxxxxxxxxxxx",
						StartTime: timeFilled,
						EndTime:   timeFilled,
					},
				},
			},
		},
		Service: xray.ServiceData,
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}

	traceHeader := <-ch
	if traceHeader.TraceID != got.TraceID {
		t.Errorf("invalid trace id, want %s, got %s", got.TraceID, traceHeader.TraceID)
	}
	if traceHeader.ParentID != got.ID {
		t.Errorf("invalid parent id, want %s, got %s", got.ID, traceHeader.ParentID)
	}
}

func TestClient_StatusTooManyRequests(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	ch := make(chan xray.TraceHeader, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceHeader := xray.ParseTraceHeader(r.Header.Get(xray.TraceIDHeaderKey))
		ch <- traceHeader
		w.WriteHeader(http.StatusTooManyRequests)
		if _, err := w.Write([]byte("hello")); err != nil {
			panic(err)
		}
	}))
	defer ts.Close()
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}

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
		Name:      "test",
		ID:        "xxxxxxxxxxxxxxxx",
		TraceID:   "x-xxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx",
		StartTime: timeFilled,
		EndTime:   timeFilled,
		Subsegments: []*schema.Segment{
			{
				Name:      "example.com",
				Namespace: "remote",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
				HTTP: &schema.HTTP{
					Request: &schema.HTTPRequest{
						Method: http.MethodGet,
						URL:    ts.URL,
					},
					Response: &schema.HTTPResponse{
						Status:        http.StatusTooManyRequests,
						ContentLength: 5,
					},
				},
				Error:    true,
				Throttle: true,
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
					{
						Name:      "response",
						ID:        "xxxxxxxxxxxxxxxx",
						StartTime: timeFilled,
						EndTime:   timeFilled,
					},
				},
			},
		},
		Service: xray.ServiceData,
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}

	traceHeader := <-ch
	if traceHeader.TraceID != got.TraceID {
		t.Errorf("invalid trace id, want %s, got %s", got.TraceID, traceHeader.TraceID)
	}
	if traceHeader.ParentID != got.ID {
		t.Errorf("invalid parent id, want %s, got %s", got.ID, traceHeader.ParentID)
	}
}

func TestClient_StatusInternalServerError(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	ch := make(chan xray.TraceHeader, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceHeader := xray.ParseTraceHeader(r.Header.Get(xray.TraceIDHeaderKey))
		ch <- traceHeader
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := w.Write([]byte("hello")); err != nil {
			panic(err)
		}
	}))
	defer ts.Close()
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}

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
		Name:      "test",
		ID:        "xxxxxxxxxxxxxxxx",
		TraceID:   "x-xxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx",
		StartTime: timeFilled,
		EndTime:   timeFilled,
		Subsegments: []*schema.Segment{
			{
				Name:      "example.com",
				Namespace: "remote",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
				HTTP: &schema.HTTP{
					Request: &schema.HTTPRequest{
						Method: http.MethodGet,
						URL:    ts.URL,
					},
					Response: &schema.HTTPResponse{
						Status:        http.StatusInternalServerError,
						ContentLength: 5,
					},
				},
				Fault: true,
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
					{
						Name:      "response",
						ID:        "xxxxxxxxxxxxxxxx",
						StartTime: timeFilled,
						EndTime:   timeFilled,
					},
				},
			},
		},
		Service: xray.ServiceData,
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}

	traceHeader := <-ch
	if traceHeader.TraceID != got.TraceID {
		t.Errorf("invalid trace id, want %s, got %s", got.TraceID, traceHeader.TraceID)
	}
	if traceHeader.ParentID != got.ID {
		t.Errorf("invalid parent id, want %s, got %s", got.ID, traceHeader.ParentID)
	}
}

func TestClient_TLS(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("hello")); err != nil {
			panic(err)
		}
	}))
	defer ts.Close()
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}

	func() {
		// lock the tls version and cipher suites for testing
		client := ts.Client()
		if t, ok := client.Transport.(*http.Transport); ok {
			t.TLSClientConfig.MinVersion = tls.VersionTLS12
			t.TLSClientConfig.MaxVersion = tls.VersionTLS12
			t.TLSClientConfig.CipherSuites = []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}
		}

		client = Client(ts.Client())

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
		Name:      "test",
		ID:        "xxxxxxxxxxxxxxxx",
		TraceID:   "x-xxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx",
		StartTime: timeFilled,
		EndTime:   timeFilled,
		Subsegments: []*schema.Segment{
			{
				Name:      "example.com",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
				Namespace: "remote",
				HTTP: &schema.HTTP{
					Request: &schema.HTTPRequest{
						Method: http.MethodGet,
						URL:    ts.URL,
					},
					Response: &schema.HTTPResponse{
						Status:        http.StatusOK,
						ContentLength: 5,
					},
				},
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
							{
								Name:      "tls",
								ID:        "xxxxxxxxxxxxxxxx",
								StartTime: timeFilled,
								EndTime:   timeFilled,
								Metadata: map[string]interface{}{
									"http": map[string]interface{}{
										"tls": map[string]interface{}{
											"version":                       "tls1.2",
											"negotiated_protocol_is_mutual": true,
											"cipher_suite":                  "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
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
					{
						Name:      "response",
						ID:        "xxxxxxxxxxxxxxxx",
						StartTime: timeFilled,
						EndTime:   timeFilled,
					},
				},
			},
		},
		Service: xray.ServiceData,
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestClient_DNS(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("hello")); err != nil {
			panic(err)
		}
	}))
	defer ts.Close()

	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}

	// Specify IP version to avoid falling back
	addr := u.Hostname()
	network := "tcp6"
	if net.ParseIP(addr).To4() != nil {
		network = "tcp4"
	}
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
				return new(net.Dialer).DialContext(ctx, network, addr)
			},
		},
	}

	u.Host = net.JoinHostPort("loopback.shogo82148.com", u.Port())
	func() {
		client := Client(client)
		ctx, root := xray.BeginSegment(ctx, "test")
		defer root.Close()
		req, err := http.NewRequest(http.MethodGet, u.String(), nil)
		if err != nil {
			t.Fatal(err)
		}
		req = req.WithContext(ctx)
		req.Host = "example.com"
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "hello" {
			t.Errorf("want %q, got %q", "hello", string(data))
		}
	}()

	got, err := td.Recv()
	if err != nil {
		t.Fatal(err)
	}
	dns := map[string]interface{}{
		"addresses": []interface{}{addr},
		"coalesced": false,
	}
	want := &schema.Segment{
		Name:      "test",
		ID:        "xxxxxxxxxxxxxxxx",
		TraceID:   "x-xxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx",
		StartTime: timeFilled,
		EndTime:   timeFilled,
		Subsegments: []*schema.Segment{
			{
				Name:      "example.com",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
				Namespace: "remote",
				HTTP: &schema.HTTP{
					Request: &schema.HTTPRequest{
						Method: http.MethodGet,
						URL:    u.String(),
					},
					Response: &schema.HTTPResponse{
						Status:        http.StatusOK,
						ContentLength: 5,
					},
				},
				Subsegments: []*schema.Segment{
					{
						Name:      "connect",
						ID:        "xxxxxxxxxxxxxxxx",
						StartTime: timeFilled,
						EndTime:   timeFilled,
						Subsegments: []*schema.Segment{
							{
								Name:      "dns",
								ID:        "xxxxxxxxxxxxxxxx",
								StartTime: timeFilled,
								EndTime:   timeFilled,
								Metadata: map[string]interface{}{
									"http": map[string]interface{}{
										"dns": dns,
									},
								},
							},
							{
								Name:      "dial",
								ID:        "xxxxxxxxxxxxxxxx",
								StartTime: timeFilled,
								EndTime:   timeFilled,
								Metadata: map[string]interface{}{
									"http": map[string]interface{}{
										"dial": map[string]interface{}{
											"network": network,
											"address": net.JoinHostPort(addr, u.Port()),
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
					{
						Name:      "response",
						ID:        "xxxxxxxxxxxxxxxx",
						StartTime: timeFilled,
						EndTime:   timeFilled,
					},
				},
			},
		},
		Service: xray.ServiceData,
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		dns["addresses"] = []interface{}{"::1", addr} // addresses may contains IPv6
		if diff2 := cmp.Diff(want, got, ignoreVariableField); diff2 != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	}
}

func TestClient_InvalidDomain(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	var httpErr error
	func() {
		client := Client(nil)
		ctx, root := xray.BeginSegment(ctx, "test")
		defer root.Close()
		req, err := http.NewRequest(http.MethodGet, "https://domain.invalid", nil)
		if err != nil {
			t.Fatal(err)
		}
		req = req.WithContext(ctx)
		resp, err := client.Do(req)
		if err != nil {
			httpErr = err
			return
		}
		defer resp.Body.Close()
		t.Fatal("want error, but not")
	}()

	got, err := td.Recv()
	if err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	urlErr, ok := httpErr.(*url.Error)
	if !ok {
		t.Fatal(httpErr)
	}
	opErr, ok := urlErr.Err.(*net.OpError)
	if !ok {
		t.Fatal(urlErr)
	}

	want := &schema.Segment{
		Name:      "test",
		ID:        "xxxxxxxxxxxxxxxx",
		TraceID:   "x-xxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx",
		StartTime: timeFilled,
		EndTime:   timeFilled,
		Subsegments: []*schema.Segment{
			{
				Name:      "domain.invalid",
				Namespace: "remote",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
				HTTP: &schema.HTTP{
					Request: &schema.HTTPRequest{
						Method: http.MethodGet,
						URL:    "https://domain.invalid",
					},
				},
				Fault: true,
				Cause: &schema.Cause{
					WorkingDirectory: wd,
					Exceptions: []schema.Exception{
						{
							ID:      "xxxxxxxxxxxxxxxx",
							Message: opErr.Error(),
							Type:    "*net.OpError",
						},
					},
				},
				Subsegments: []*schema.Segment{
					{
						Name:      "connect",
						ID:        "xxxxxxxxxxxxxxxx",
						StartTime: timeFilled,
						EndTime:   timeFilled,
						Fault:     true,
						Subsegments: []*schema.Segment{
							{
								Name:      "dns",
								ID:        "xxxxxxxxxxxxxxxx",
								StartTime: timeFilled,
								EndTime:   timeFilled,
								Fault:     true,
								Cause: &schema.Cause{
									WorkingDirectory: wd,
									Exceptions: []schema.Exception{
										{
											ID:      "xxxxxxxxxxxxxxxx",
											Message: opErr.Err.Error(),
											Type:    "*net.DNSError",
										},
									},
								},
								Metadata: map[string]interface{}{
									"http": map[string]interface{}{
										"dns": map[string]interface{}{
											"addresses": []interface{}{},
											"coalesced": false,
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
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestClient_InvalidAddress(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	var httpErr error
	func() {
		client := Client(nil)
		ctx, root := xray.BeginSegment(ctx, "test")
		defer root.Close()

		// we expected no Gopher daemon on this computer ʕ◔ϖ◔ʔ
		req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:70", nil)
		if err != nil {
			t.Fatal(err)
		}
		req = req.WithContext(ctx)
		resp, err := client.Do(req)
		if err != nil {
			httpErr = err
			return
		}
		defer resp.Body.Close()
		t.Fatal("want error, but not")
	}()

	got, err := td.Recv()
	if err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	urlErr, ok := httpErr.(*url.Error)
	if !ok {
		t.Fatal(httpErr)
	}
	opErr, ok := urlErr.Err.(*net.OpError)
	if !ok {
		t.Fatal(urlErr)
	}

	want := &schema.Segment{
		Name:      "test",
		ID:        "xxxxxxxxxxxxxxxx",
		TraceID:   "x-xxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx",
		StartTime: timeFilled,
		EndTime:   timeFilled,
		Subsegments: []*schema.Segment{
			{
				Name:      "127.0.0.1:70",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
				Namespace: "remote",
				HTTP: &schema.HTTP{
					Request: &schema.HTTPRequest{
						Method: http.MethodGet,
						URL:    "http://127.0.0.1:70",
					},
				},
				Fault: true,
				Cause: &schema.Cause{
					WorkingDirectory: wd,
					Exceptions: []schema.Exception{
						{
							ID:      "xxxxxxxxxxxxxxxx",
							Message: opErr.Error(),
							Type:    "*net.OpError",
						},
					},
				},
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
											Message: opErr.Error(),
											Type:    "*net.OpError",
										},
									},
								},
								Metadata: map[string]interface{}{
									"http": map[string]interface{}{
										"dial": map[string]interface{}{
											"address": "127.0.0.1:70",
											"network": "tcp",
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
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestClient_InvalidCertificate(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("hello")); err != nil {
			panic(err)
		}
	}))
	defer ts.Close()
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}

	var httpErr error
	func() {
		// we don't use ts.Client() here, because we want to test certificate error
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
			httpErr = err
			return
		}
		defer resp.Body.Close()
		t.Fatal("want error, but not")
	}()

	got, err := td.Recv()
	if err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	urlErr, ok := httpErr.(*url.Error)
	if !ok {
		t.Fatal(httpErr)
	}

	want := &schema.Segment{
		Name:      "test",
		TraceID:   "x-xxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx",
		ID:        "xxxxxxxxxxxxxxxx",
		StartTime: timeFilled,
		EndTime:   timeFilled,
		Subsegments: []*schema.Segment{
			{
				Name:      "example.com",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
				Namespace: "remote",
				HTTP: &schema.HTTP{
					Request: &schema.HTTPRequest{
						Method: http.MethodGet,
						URL:    ts.URL,
					},
				},
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
								Metadata: map[string]interface{}{
									"http": map[string]interface{}{
										"dial": map[string]interface{}{
											"address": u.Host,
											"network": "tcp",
										},
									},
								},
							},
							{
								Name:      "tls",
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
							},
						},
					},
				},
			},
		},
		Service: xray.ServiceData,
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestClient_FailToReadResponse(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if l, err = net.Listen("tcp6", "[::1]:0"); err != nil {
			panic(fmt.Sprintf("failed to listen on a port: %v", err))
		}
	}
	defer l.Close()

	go func() {
		for {
			conn, err := l.Accept()
			if e, ok := err.(net.Error); ok && e.Temporary() {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			if err != nil {
				return
			}
			go func() {
				var b [1024]byte
				conn.Read(b[:]) // ignore http request
				conn.Close()
			}()
		}
	}()

	var httpErr error
	func() {
		client := Client(nil)
		ctx, root := xray.BeginSegment(ctx, "test")
		defer root.Close()
		req, err := http.NewRequest(http.MethodGet, "http://"+l.Addr().String(), nil)
		if err != nil {
			t.Fatal(err)
		}
		req = req.WithContext(ctx)
		req.Host = "example.com"
		resp, err := client.Do(req)
		if err != nil {
			httpErr = err
			return
		}
		defer resp.Body.Close()
		t.Fatal("want error, but not")
	}()

	got, err := td.Recv()
	if err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	urlErr, ok := httpErr.(*url.Error)
	if !ok {
		t.Fatal(httpErr)
	}

	want := &schema.Segment{
		Name:      "test",
		ID:        "xxxxxxxxxxxxxxxx",
		TraceID:   "x-xxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx",
		StartTime: timeFilled,
		EndTime:   timeFilled,
		Subsegments: []*schema.Segment{
			{
				Name:      "example.com",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
				Namespace: "remote",
				HTTP: &schema.HTTP{
					Request: &schema.HTTPRequest{
						Method: http.MethodGet,
						URL:    "http://" + l.Addr().String(),
					},
				},
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
											"network": l.Addr().Network(),
											"address": l.Addr().String(),
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
		},
		Service: xray.ServiceData,
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestClient_UnexpectedEOF(t *testing.T) {
	ctx, td := xray.NewTestDaemon(nil)
	defer td.Close()

	ch := make(chan xray.TraceHeader, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceHeader := xray.ParseTraceHeader(r.Header.Get(xray.TraceIDHeaderKey))
		ch <- traceHeader
		w.Header().Set("Content-Length", "5")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("hell")); err != nil {
			panic(err)
		}
	}))
	defer ts.Close()
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}

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
		if err != io.ErrUnexpectedEOF {
			t.Fatal(err)
		}
		if string(data) != "hell" {
			t.Errorf("want %q, got %q", "hello", string(data))
		}
	}()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

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
		Subsegments: []*schema.Segment{
			{
				Name:      "example.com",
				Namespace: "remote",
				ID:        "xxxxxxxxxxxxxxxx",
				StartTime: timeFilled,
				EndTime:   timeFilled,
				HTTP: &schema.HTTP{
					Request: &schema.HTTPRequest{
						Method: http.MethodGet,
						URL:    ts.URL,
					},
					Response: &schema.HTTPResponse{
						Status:        http.StatusOK,
						ContentLength: 5,
					},
				},
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
					{
						Name:      "response",
						ID:        "xxxxxxxxxxxxxxxx",
						StartTime: timeFilled,
						EndTime:   timeFilled,
						Fault:     true,
						Cause: &schema.Cause{
							WorkingDirectory: wd,
							Exceptions: []schema.Exception{
								{
									ID:      "xxxxxxxxxxxxxxxx",
									Message: io.ErrUnexpectedEOF.Error(),
									Type:    "*errors.errorString",
								},
							},
						},
					},
				},
			},
		},
		Service: xray.ServiceData,
	}
	if diff := cmp.Diff(want, got, ignoreVariableField); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}

	traceHeader := <-ch
	if traceHeader.TraceID != got.TraceID {
		t.Errorf("invalid trace id, want %s, got %s", got.TraceID, traceHeader.TraceID)
	}
	if traceHeader.ParentID != got.ID {
		t.Errorf("invalid parent id, want %s, got %s", got.ID, traceHeader.ParentID)
	}
}
