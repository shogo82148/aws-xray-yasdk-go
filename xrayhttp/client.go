package xrayhttp

import (
	"net/http"

	"github.com/shogo82148/aws-xray-yasdk-go/xray"
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

	ctx, seg := xray.BeginSubsegment(req.Context(), host)
	defer seg.Close()
	if !isEmptyHost {
		seg.SetNamespace("remote")
	}

	ctx = WithClientTrace(ctx)
	req = req.WithContext(ctx)
	resp, err := rt.Base.RoundTrip(req)
	seg.AddError(err)
	return resp, err
}
