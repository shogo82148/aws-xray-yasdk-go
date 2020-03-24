package xrayhttp

import (
	"context"
	"crypto/tls"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
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
		Name: "test",
		Subsegments: []*schema.Segment{
			{
				Name:      "example.com",
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
		Name: "test",
		Subsegments: []*schema.Segment{
			{
				Name:      "example.com",
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
							{
								Name: "tls",
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
					{Name: "request"},
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
		Name: "test",
		Subsegments: []*schema.Segment{
			{
				Name:      "example.com",
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
						Name: "connect",
						Subsegments: []*schema.Segment{
							{
								Name: "dns",
								Metadata: map[string]interface{}{
									"http": map[string]interface{}{
										"dns": dns,
									},
								},
							},
							{
								Name: "dial",
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
					{Name: "request"},
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
		Name: "test",
		Subsegments: []*schema.Segment{
			{
				Name:      "domain.invalid",
				Namespace: "remote",
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
							Message: opErr.Error(),
							Type:    "*net.OpError",
						},
					},
				},
				Subsegments: []*schema.Segment{
					{
						Name:  "connect",
						Fault: true,
						Subsegments: []*schema.Segment{
							{
								Name:  "dns",
								Fault: true,
								Cause: &schema.Cause{
									WorkingDirectory: wd,
									Exceptions: []schema.Exception{
										{
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
