package xrayhttp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"sync"

	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

const emptyHostRename = "empty_host_error"

// Client creates a shallow copy of the provided http client,
// defaulting to http.DefaultClient, with roundtripper wrapped
// with xrayhttp.RoundTripper.
func Client(client *http.Client) *http.Client {
	if client == nil {
		client = http.DefaultClient
	}
	ret := *client
	ret.Transport = RoundTripper(ret.Transport)
	return &ret
}

// RoundTripper wraps the provided http roundtripper with xray.Capture,
// sets HTTP-specific xray fields, and adds the trace header to the outbound request.
func RoundTripper(rt http.RoundTripper) http.RoundTripper {
	if rt == nil {
		rt = http.DefaultTransport
	}
	if _, ok := rt.(*roundtripper); ok {
		// X-Ray SDK is already installed
		return rt
	}
	return &roundtripper{
		Base: rt,
	}
}

type roundtripper struct {
	Base http.RoundTripper
}

func (rt *roundtripper) RoundTrip(req *http.Request) (*http.Response, error) {
	var isEmptyHost bool
	host := req.Host
	if host == "" {
		if h := req.URL.Host; h != "" {
			host = h
		} else {
			host = emptyHostRename
			isEmptyHost = true
		}
	}

	ctx := req.Context()
	req.Header.Set(xray.TraceIDHeaderKey, xray.DownstreamHeader(ctx).String())

	ctx, seg := xray.BeginSubsegment(ctx, host)
	defer seg.Close()
	if !isEmptyHost {
		seg.SetNamespace("remote")
	}

	requestInfo := &schema.HTTPRequest{
		Method: req.Method,
		URL:    req.URL.String(),
	}
	seg.SetHTTPRequest(requestInfo)

	// set trace hooks
	ctx, cancel := WithClientTrace(ctx)
	defer cancel()
	respTracer := &clientResponseTracer{BaseContext: ctx}
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		GotFirstResponseByte: respTracer.GotFirstResponseByte,
	})
	req = req.WithContext(ctx)

	resp, err := rt.Base.RoundTrip(req)
	if err != nil {
		seg.AddError(err)
		respTracer.Close()
		return nil, err
	}

	responseInfo := &schema.HTTPResponse{
		Status: resp.StatusCode,
	}
	if length, err := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64); err == nil {
		responseInfo.ContentLength = length
	}
	seg.SetHTTPResponse(responseInfo)
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		seg.SetError()
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		seg.SetThrottle()
	}
	if resp.StatusCode >= 500 && resp.StatusCode < 600 {
		seg.SetFault()
	}
	if resp.StatusCode == http.StatusSwitchingProtocols {
		respTracer.Close()
	} else {
		respTracer.body = resp.Body
		resp.Body = respTracer
	}
	return resp, err
}

type clientResponseTracer struct {
	BaseContext context.Context
	mu          sync.RWMutex
	body        io.ReadCloser
	ctx         context.Context
	seg         *xray.Segment
}

func (r *clientResponseTracer) GotFirstResponseByte() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ctx != nil {
		return
	}
	r.ctx, r.seg = xray.BeginSubsegment(r.BaseContext, "response")
}

func (r *clientResponseTracer) Read(b []byte) (int, error) {
	r.mu.RLock()
	body := r.body
	r.mu.RUnlock()
	if body != nil {
		return body.Read(b)
	}
	return 0, io.EOF
}

func (r *clientResponseTracer) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var err error
	if r.body != nil {
		err = r.body.Close()
	}
	if r.ctx != nil {
		r.seg.Close()
		r.ctx, r.seg = nil, nil
	}
	return err
}
