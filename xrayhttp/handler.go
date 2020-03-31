package xrayhttp

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/sampling"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

//go:generate go run codegen.go

// TracingNamer is the interface for naming service node.
// If it returns empty string, the value of AWS_XRAY_TRACING_NAME environment value is used.
type TracingNamer interface {
	TracingName(r *http.Request) string
}

// FixedTracingNamer records the fixed name of service node.
type FixedTracingNamer string

// TracingName implements TracingNamer.
func (tn FixedTracingNamer) TracingName(r *http.Request) string {
	return string(tn)
}

// DynamicTracingNamer chooses names for segments generated
// for incoming requests by parsing the HOST header of the
// incoming request. If the host header matches a given
// recognized pattern (using the included pattern package),
// it is used as the segment name. Otherwise, the fallback
// name is used.
type DynamicTracingNamer struct {
	RecognizedHosts string
	FallbackName    string
}

// TracingName implements TracingNamer.
func (tn DynamicTracingNamer) TracingName(r *http.Request) string {
	if sampling.WildcardMatchCaseInsensitive(tn.RecognizedHosts, r.Host) {
		return r.Host
	}
	return tn.FallbackName
}

type httpTracer struct {
	tn     TracingNamer
	client *xray.Client
	h      http.Handler
}

// Handler wraps the provided http handler with xray.Capture
func Handler(tn TracingNamer, h http.Handler) http.Handler {
	return &httpTracer{
		tn: tn,
		h:  h,
	}
}

// HandlerWithClient wraps the provided http handler with xray.Capture
func HandlerWithClient(tn TracingNamer, client *xray.Client, h http.Handler) http.Handler {
	return &httpTracer{
		tn:     tn,
		client: client,
		h:      h,
	}
}

func (tracer *httpTracer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := tracer.tn.TracingName(r)
	if name == "" {
		name = os.Getenv("AWS_XRAY_TRACING_NAME")
		if name == "" {
			name = "unknown"
		}
	}
	ctx := r.Context()
	if tracer.client != nil {
		ctx = xray.WithClient(ctx, tracer.client)
	}
	ctx, seg := xray.BeginSegmentWithRequest(ctx, name, r)
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

	rw := &serverResponseTracer{rw: w, ctx: ctx, seg: seg}
	defer rw.close()
	tracer.h.ServeHTTP(wrap(rw), r)
	if rw.hijacked {
		return
	}

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

// backport of io.StringWriter from Go 1.11
type stringWriter interface {
	WriteString(s string) (n int, err error)
}

type responseWriter interface {
	http.ResponseWriter
	io.ReaderFrom
	stringWriter
}

type serverResponseTracer struct {
	ctx      context.Context
	seg      *xray.Segment
	respCtx  context.Context
	respSeg  *xray.Segment
	rw       http.ResponseWriter
	status   int
	size     int64
	hijacked bool
}

func (rw *serverResponseTracer) Header() http.Header {
	return rw.rw.Header()
}

func (rw *serverResponseTracer) Write(b []byte) (int, error) {
	if rw.status == 0 {
		rw.WriteHeader(http.StatusOK)
	}
	size, err := rw.rw.Write(b)
	rw.size += int64(size)
	return size, err
}

func (rw *serverResponseTracer) WriteHeader(s int) {
	if rw.respCtx == nil {
		rw.respCtx, rw.respSeg = xray.BeginSubsegment(rw.ctx, "response")
	}
	rw.rw.WriteHeader(s)
	rw.status = s
}

func (rw *serverResponseTracer) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h := rw.rw.(http.Hijacker)
	conn, buf, err := h.Hijack()
	if err == nil {
		if rw.status == 0 {
			// The status will be StatusSwitchingProtocols if there was no error and
			// WriteHeader has not been called yet
			rw.status = http.StatusSwitchingProtocols
		}
		rw.hijacked = true
		responseInfo := &schema.HTTPResponse{
			Status:        rw.status,
			ContentLength: rw.size,
		}
		rw.seg.SetHTTPResponse(responseInfo)
		rw.close()
	}
	return conn, buf, err
}

func (rw *serverResponseTracer) Flush() {
	if f, ok := rw.rw.(http.Flusher); ok {
		f.Flush()
	}
}

func (rw *serverResponseTracer) Push(target string, opts *http.PushOptions) error {
	if p, ok := rw.rw.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}

func (rw *serverResponseTracer) CloseNotify() <-chan bool {
	n := rw.rw.(http.CloseNotifier)
	return n.CloseNotify()
}

func (rw *serverResponseTracer) WriteString(str string) (int, error) {
	var size int
	var err error
	if s, ok := rw.rw.(stringWriter); ok {
		size, err = s.WriteString(str)
	} else {
		size, err = rw.rw.Write([]byte(str))
	}
	rw.size += int64(size)
	return size, err
}

func (rw *serverResponseTracer) ReadFrom(src io.Reader) (int64, error) {
	var size int64
	var err error
	if r, ok := rw.rw.(io.ReaderFrom); ok {
		size, err = r.ReadFrom(src)
	} else {
		size, err = io.Copy(rw.rw, src)
	}
	rw.size += size
	return size, err
}

func (rw *serverResponseTracer) close() {
	err := recover()
	if rw.respCtx != nil {
		if err != nil {
			rw.respSeg.SetFault()
		}
		rw.respSeg.Close()
		rw.respCtx, rw.respSeg = nil, nil
	}
	if rw.ctx != nil {
		rw.seg.AddPanic(err)
		rw.seg.Close()
		rw.ctx, rw.seg = nil, nil
	}
	if err != nil {
		panic(err)
	}
}
