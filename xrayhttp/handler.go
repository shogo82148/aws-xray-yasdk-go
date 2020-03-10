package xrayhttp

import (
	"net"
	"net/http"
	"strings"

	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

// TracingNamer is the interface for naming service node.
type TracingNamer interface {
	TracingName(r *http.Request) string
}

// FixedTracingNamer records the fixed name of service node.
type FixedTracingNamer string

// TracingName implements TracingNamer.
func (tn FixedTracingNamer) TracingName(r *http.Request) string {
	return string(tn)
}

type httpTracer struct {
	tn TracingNamer
	h  http.Handler
}

// Handler wraps the provided http handler with xray.Capture
func Handler(tn TracingNamer, h http.Handler) http.Handler {
	return &httpTracer{
		tn: tn,
		h:  h,
	}
}

func (tracer *httpTracer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := tracer.tn.TracingName(r)
	header := xray.ParseTraceHeader(r.Header.Get(xray.TraceIDHeaderKey))
	ctx, seg := xray.NewSegmentFromHeader(r.Context(), name, r, header)
	defer seg.Close()
	r = r.WithContext(ctx)

	ip, forwarded := clientIP(r)
	requestInfo := &schema.HTTPRequest{
		Method:        r.Method,
		URL:           getURL(r),
		ClientIP:      ip,
		XForwardedFor: forwarded,
		UserAgent:     r.UserAgent(),
	}
	seg.SetHTTPRequest(requestInfo)

	rw := &responseTracer{rw: w}
	tracer.h.ServeHTTP(rw, r)

	responseInfo := &schema.HTTPResponse{
		Status:        rw.status,
		ContentLength: rw.size,
	}
	seg.SetHTTPResponse(responseInfo)
	if rw.status >= 400 && rw.status < 500 {
		seg.SetError()
	}
	if rw.status == http.StatusTooManyRequests {
		seg.SetThrottle()
	}
	if rw.status >= 500 && rw.status < 600 {
		seg.SetFault()
	}
}

func getURL(r *http.Request) string {
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	return proto + "://" + r.Host + r.URL.Path
}

func clientIP(r *http.Request) (string, bool) {
	forwardedFor := r.Header.Get("X-Forwarded-For")
	if forwardedFor != "" {
		if idx := strings.IndexByte(forwardedFor, ','); idx > 0 {
			forwardedFor = forwardedFor[:idx]
		}
		return strings.TrimSpace(forwardedFor), true
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr, false
	}
	return ip, false
}

type responseTracer struct {
	rw     http.ResponseWriter
	status int
	size   int
}

func (rw *responseTracer) Header() http.Header {
	return rw.rw.Header()
}

func (rw *responseTracer) Write(b []byte) (int, error) {
	if rw.status == 0 {
		rw.WriteHeader(http.StatusOK)
	}
	size, err := rw.rw.Write(b)
	rw.size += size
	return size, err
}

func (rw *responseTracer) WriteHeader(s int) {
	rw.rw.WriteHeader(s)
	rw.status = s
}
