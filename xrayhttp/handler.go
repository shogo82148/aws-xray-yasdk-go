package xrayhttp

import (
	"net/http"

	"github.com/shogo82148/aws-xray-yasdk-go/xray"
)

// TracingNamer is the interface for naming service node.
type TracingNamer interface {
	TracingName(r *http.Request) string
}

// FixedTracingNamer records the fixed name of service node.
type FixedTracingNamer string

// TracingName implements TracingNamer.
func (tn FixedTracingNamer) TracingName(r *http.Request) string {
	return string(sn)
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
	tracer.h.ServeHTTP(w, r)
}
