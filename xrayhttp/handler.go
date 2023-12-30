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
	forwardedheader "github.com/shogo82148/forwarded-header"
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

// Handler wraps the provided [net/http.Handler].
// The returned [net/http.Handler] creates a sub-segment and collects information of the request.
func Handler(tn TracingNamer, h http.Handler) http.Handler {
	return &httpTracer{
		tn: tn,
		h:  h,
	}
}

// HandlerWithClient wraps the provided [net/http.Handler].
// The returned [net/http.Handler] creates a sub-segment and collects information of the request.
func HandlerWithClient(tn TracingNamer, client *xray.Client, h http.Handler) http.Handler {
	return &httpTracer{
		tn:     tn,
		client: client,
		h:      h,
	}
}

// ServeHTTP implements [net/http.Handler].
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

	// Set error flag if http connection is already closed by client.
	select {
	case <-ctx.Done():
		seg.SetError()
		return
	default:
	}

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
	forwarded, err := forwardedheader.Parse(r.Header.Values("Forwarded"))
	if err == nil && len(forwarded) > 0 {
		proto := forwarded[0].Proto
		host := forwarded[0].Host
		if proto != "" && host != "" {
			return proto + "://" + host + r.URL.Path
		}
	}
	proto := r.Header.Get("X-Forwarded-Proto")
	if strings.EqualFold(proto, "https") {
		proto = "https"
	} else if strings.EqualFold(proto, "http") {
		proto = "http"
	} else if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	return proto + "://" + r.Host + r.URL.Path
}

func clientIP(r *http.Request) (string, bool) {
	forwarded, err := forwardedheader.Parse(r.Header.Values("Forwarded"))
	if err == nil && len(forwarded) > 0 {
		ip := forwarded[0].For.IP
		if ip.IsValid() {
			return ip.String(), true
		}
	}
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

type rwUnwrapper interface {
	// Unwrap returns the original http.ResponseWriter underlying this.
	// It is used by [net/http.ResponseController].
	Unwrap() http.ResponseWriter
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

// Header implements [net/http.ResponseWriter].
func (rw *serverResponseTracer) Header() http.Header {
	return rw.rw.Header()
}

// Write implements [net/http.ResponseWriter].
func (rw *serverResponseTracer) Write(b []byte) (int, error) {
	if rw.status == 0 {
		rw.WriteHeader(http.StatusOK)
	}
	size, err := rw.rw.Write(b)
	rw.size += int64(size)
	return size, err
}

// WriteHeader implements [net/http.ResponseWriter].
func (rw *serverResponseTracer) WriteHeader(s int) {
	if rw.respCtx == nil {
		rw.respCtx, rw.respSeg = xray.BeginSubsegment(rw.ctx, "response")
	}
	rw.rw.WriteHeader(s)
	rw.status = s
}

// Hijack implements [net/http.Hijacker].
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

// Flush implements [net/http.Flusher].
func (rw *serverResponseTracer) Flush() {
	// we don't check rw.rw actually implements http.Flusher here, because it is done in the wrap func.
	f := rw.rw.(http.Flusher)
	f.Flush()
}

// Push implements [net/http.Pusher].
func (rw *serverResponseTracer) Push(target string, opts *http.PushOptions) error {
	// we don't check rw.rw actually implements http.Pusher here, because it is done in the wrap func.
	p := rw.rw.(http.Pusher)
	return p.Push(target, opts)
}

func (rw *serverResponseTracer) CloseNotify() <-chan bool {
	// we don't check rw.rw actually implements http.CloseNotifier here, because it is done in the wrap func.
	n := rw.rw.(http.CloseNotifier)
	return n.CloseNotify()
}

// WriteString implements [io.StringWriter].
func (rw *serverResponseTracer) WriteString(str string) (int, error) {
	// we don't check rw.rw actually implements io.StringWriter here, because it is done in the wrap func.
	s := rw.rw.(io.StringWriter)
	size, err := s.WriteString(str)
	rw.size += int64(size)
	return size, err
}

// ReadFrom implements [io.ReaderFrom].
func (rw *serverResponseTracer) ReadFrom(src io.Reader) (int64, error) {
	// we don't check rw.rw actually implements io.ReaderFrom here, because it is done in the wrap func.
	r := rw.rw.(io.ReaderFrom)
	size, err := r.ReadFrom(src)
	rw.size += size
	return size, err
}

// Unwrap returns the original http.ResponseWriter underlying this.
// It is used by [net/http.ResponseController].
func (rw *serverResponseTracer) Unwrap() http.ResponseWriter {
	return rw.rw
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
