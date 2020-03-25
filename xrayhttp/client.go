package xrayhttp

import (
	"net/http"
	"strconv"

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

	ctx, cancel := WithClientTrace(ctx)
	defer cancel()
	req = req.WithContext(ctx)
	resp, err := rt.Base.RoundTrip(req)
	if err != nil {
		seg.AddError(err)
		return nil, err
	}

	responseInfo := &schema.HTTPResponse{
		Status: resp.StatusCode,
	}
	if length, err := strconv.Atoi(resp.Header.Get("Content-Length")); err == nil {
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
	return resp, err
}
