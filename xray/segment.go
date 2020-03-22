package xray

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/shogo82148/aws-xray-yasdk-go/xray/sampling"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/xraylog"
)

var nowFunc func() time.Time = time.Now

// contextKey is a value for use with context.WithValue. It's used as
// a pointer so it fits in an interface{} without allocation.
type contextKey struct {
	name string
}

func (k *contextKey) String() string { return "xray context value " + k.name }

var (
	segmentContextKey = &contextKey{"segment"}
	clientContextKey  = &contextKey{"client"}
)

type segmentStatus int

const (
	segmentStatusInit segmentStatus = iota
	segmentStatusEmitted
)

// Segment is a segment.
type Segment struct {
	mu        sync.RWMutex
	ctx       context.Context
	name      string
	id        string
	traceID   string
	startTime time.Time
	endTime   time.Time
	status    segmentStatus
	sampled   bool

	// parent segment
	// if the segment is the root, the parent is nil.
	parent *Segment

	// set by NewSegmentFromHeader
	traceHeader TraceHeader

	// root segment
	// if the segment is the root, the root points the segment it self.
	root *Segment

	// subsegments that are not completed.
	subsegments []*Segment

	// statics of the subsegments, used in the root.
	totalSegments   int
	closedSegments  int
	emittedSegments int

	// error information
	error    bool
	throttle bool
	fault    bool
	cause    *schema.Cause

	namespace string
	metadata  map[string]interface{}
	sql       *schema.SQL
	http      *schema.HTTP
}

// NewTraceID generates a string format of random trace ID.
func NewTraceID() string {
	var r [12]byte
	_, err := rand.Read(r[:])
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("1-%08x-%x", nowFunc().Unix(), r)
}

// NewSegmentID generates a string format of segment ID.
func NewSegmentID() string {
	var r [8]byte
	_, err := rand.Read(r[:])
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", r)
}

// ContextSegment return the segment of current context.
func ContextSegment(ctx context.Context) *Segment {
	seg := ctx.Value(segmentContextKey)
	if seg == nil {
		return nil
	}
	return seg.(*Segment)
}

// WithSegment returns a new context with the existing segment.
func WithSegment(ctx context.Context, seg *Segment) context.Context {
	return context.WithValue(ctx, segmentContextKey, seg)
}

// BeginSegment creates a new Segment for a given name and context.
//
// Caller should close the segment when the work is done.
func BeginSegment(ctx context.Context, name string) (context.Context, *Segment) {
	return BeginSegmentWithRequest(ctx, name, nil)
}

// BeginSegmentWithRequest creates a new Segment for a given name and context.
//
// Caller should close the segment when the work is done.
func BeginSegmentWithRequest(ctx context.Context, name string, r *http.Request) (context.Context, *Segment) {
	seg := &Segment{
		ctx:           ctx,
		name:          name, // TODO: @shogo82148 sanitize the name
		id:            NewSegmentID(),
		startTime:     nowFunc(),
		totalSegments: 1,
	}
	seg.root = seg

	var h TraceHeader
	if r != nil {
		// Sampling strategy for http calls
		h = ParseTraceHeader(r.Header.Get(TraceIDHeaderKey))
		switch h.SamplingDecision {
		case SamplingDecisionSampled:
			xraylog.Debug(ctx, "Incoming header decided: Sampled=true")
			seg.sampled = true
		case SamplingDecisionNotSampled:
			xraylog.Debug(ctx, "Incoming header decided: Sampled=false")
		default:
			client := seg.client()
			sd := client.samplingStrategy.ShouldTrace(&sampling.Request{
				Host:   r.Host,
				URL:    r.URL.Path,
				Method: r.Method,
				// TODO: ServiceName
				// TODO: ServiceType
			})
			seg.sampled = sd.Sample
			xraylog.Debugf(ctx, "SamplingStrategy decided: %t", seg.sampled)
		}
	} else {
		client := seg.client()
		sd := client.samplingStrategy.ShouldTrace(nil)
		seg.sampled = sd.Sample
		xraylog.Debugf(ctx, "SamplingStrategy decided: %t", seg.sampled)
	}
	if h.TraceID == "" {
		h.TraceID = NewTraceID()
	}
	seg.traceID = h.TraceID
	seg.traceHeader = h

	ctx = context.WithValue(ctx, segmentContextKey, seg)
	return ctx, seg
}

// BeginSubsegment creates a new Segment for a given name and context.
//
// Caller should close the segment when the work is done.
func BeginSubsegment(ctx context.Context, name string) (context.Context, *Segment) {
	now := nowFunc()

	value := ctx.Value(segmentContextKey)
	if value == nil {
		client := defaultClient
		if c := ctx.Value(clientContextKey); c != nil {
			client = c.(*Client)
		}
		client.ctxmissingStrategy.ContextMissing(ctx, "context missing for "+name)
		return ctx, nil
	}
	parent := value.(*Segment)
	if parent == nil {
		return ctx, nil
	}

	root := parent.root
	seg := &Segment{
		ctx:       ctx,
		name:      name, // TODO: @shogo82148 sanitize the name
		id:        NewSegmentID(),
		parent:    parent,
		root:      root,
		traceID:   parent.traceID,
		startTime: now,
	}
	ctx = context.WithValue(ctx, segmentContextKey, seg)

	root.mu.Lock()
	defer root.mu.Unlock()
	if parent != root {
		parent.mu.Lock()
		defer parent.mu.Unlock()
	}
	root.totalSegments++
	parent.subsegments = append(parent.subsegments, seg)

	return ctx, seg
}

// Sampled returns whether the current segment is sampled.
func (seg *Segment) Sampled() bool {
	root := seg.root
	root.mu.Lock()
	defer root.mu.Unlock()
	return root.sampled
}

type errorPanic struct {
	err interface{}
}

func (err *errorPanic) Error() string {
	return fmt.Sprintf("%T: %v", err.err, err.err)
}

// Close closes the segment.
func (seg *Segment) Close() {
	if seg.parent != nil {
		xraylog.Debugf(seg.ctx, "Closing subsegment named %s", seg.name)
	} else {
		xraylog.Debugf(seg.ctx, "Closing segment named %s", seg.name)
	}
	err := recover()
	if err != nil {
		seg.AddError(&errorPanic{err: err})
	}
	seg.close()
	if seg.Sampled() {
		seg.emit()
	}
	if err != nil {
		panic(err)
	}
}

func (seg *Segment) close() {
	root := seg.root
	root.mu.Lock()
	defer root.mu.Unlock()
	if seg != root {
		seg.mu.Lock()
		defer seg.mu.Unlock()
	}
	root.closedSegments++
	seg.endTime = nowFunc()
}

func (seg *Segment) isRoot() bool {
	return seg.parent == nil
}

func (seg *Segment) inProgress() bool {
	return seg.endTime.IsZero()
}

func (seg *Segment) emit() {
	seg.client().Emit(seg.ctx, seg)
}

func (seg *Segment) client() *Client {
	if seg == nil {
		return defaultClient
	}
	seg.mu.RLock()
	defer seg.mu.RUnlock()
	if client := seg.ctx.Value(clientContextKey); client != nil {
		return client.(*Client)
	}
	return defaultClient
}

func newExceptionID() string {
	var r [8]byte
	_, err := rand.Read(r[:])
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", r)
}

// AddError sets error.
func (seg *Segment) AddError(err error) bool {
	if seg == nil {
		return err != nil
	}
	if err == nil {
		return false
	}
	seg.mu.Lock()
	defer seg.mu.Unlock()

	seg.fault = true
	if seg.cause == nil {
		seg.cause = &schema.Cause{}
	}
	seg.cause.WorkingDirectory, _ = os.Getwd()
	seg.cause.Exceptions = append(seg.cause.Exceptions, schema.Exception{
		ID:      newExceptionID(),
		Type:    fmt.Sprintf("%T", err),
		Message: err.Error(),
	})
	return true
}

// AddError sets the segment of the current context an error.
func AddError(ctx context.Context, err error) bool {
	return ContextSegment(ctx).AddError(err)
}

// SetError sets error flag.
func (seg *Segment) SetError() {
	if seg == nil {
		return
	}
	seg.mu.Lock()
	defer seg.mu.Unlock()
	seg.error = true
}

// SetError sets error flag.
func SetError(ctx context.Context) {
	ContextSegment(ctx).SetError()
}

// SetThrottle sets throttle flag.
func (seg *Segment) SetThrottle() {
	if seg == nil {
		return
	}
	seg.mu.Lock()
	defer seg.mu.Unlock()
	seg.throttle = true
}

// SetThrottle sets error flag.
func SetThrottle(ctx context.Context) {
	ContextSegment(ctx).SetThrottle()
}

// SetFault sets fault flag.
func (seg *Segment) SetFault() {
	if seg == nil {
		return
	}
	seg.mu.Lock()
	defer seg.mu.Unlock()
	seg.fault = true
}

// SetFault sets fault flag.
func SetFault(ctx context.Context) {
	ContextSegment(ctx).SetFault()
}

// DownstreamHeader returns a header for passing to downstream calls.
func (seg *Segment) DownstreamHeader() TraceHeader {
	if seg == nil {
		return TraceHeader{}
	}
	seg.mu.RLock()
	defer seg.mu.RUnlock()
	h := seg.traceHeader
	h.TraceID = seg.traceID
	h.ParentID = seg.id
	return h
}

// DownstreamHeader returns a header for passing to downstream calls.
func DownstreamHeader(ctx context.Context) TraceHeader {
	return ContextSegment(ctx).DownstreamHeader()
}

// SetNamespace sets namespace
func (seg *Segment) SetNamespace(namespace string) {
	if seg == nil {
		return
	}
	seg.mu.Lock()
	defer seg.mu.Unlock()
	seg.namespace = namespace
}

// SetNamespace sets namespace
func SetNamespace(ctx context.Context, namespace string) {
	ContextSegment(ctx).SetNamespace(namespace)
}

// Namespace returns the namespace.
func (seg *Segment) Namespace() string {
	if seg == nil {
		return ""
	}
	seg.mu.RLock()
	defer seg.mu.RUnlock()
	return seg.namespace
}

// AddMetadata adds metadata.
func (seg *Segment) AddMetadata(key string, value interface{}) {
	seg.AddMetadataToNamespace("default", key, value)
}

// AddMetadata adds metadata.
func AddMetadata(ctx context.Context, key string, value interface{}) {
	ContextSegment(ctx).AddMetadataToNamespace("default", key, value)
}

// AddMetadataToNamespace adds metadata.
func (seg *Segment) AddMetadataToNamespace(namespace, key string, value interface{}) {
	if seg == nil {
		return
	}
	seg.mu.Lock()
	defer seg.mu.Unlock()
	if seg.metadata == nil {
		seg.metadata = map[string]interface{}{}
	}
	if seg.metadata[namespace] == nil {
		seg.metadata[namespace] = map[string]interface{}{}
	}
	if ns, ok := seg.metadata[namespace].(map[string]interface{}); ok {
		ns[key] = value
	}
}

// AddMetadataToNamespace adds metadata.
func AddMetadataToNamespace(ctx context.Context, namespace, key string, value interface{}) {
	ContextSegment(ctx).AddMetadata(key, value)
}

// SetSQL sets the information of SQL queries.
func (seg *Segment) SetSQL(sql *schema.SQL) {
	if seg == nil {
		return
	}
	seg.mu.Lock()
	defer seg.mu.Unlock()
	seg.sql = sql
}

// SetSQL sets the information of SQL queries.
func SetSQL(ctx context.Context, sql *schema.SQL) {
	ContextSegment(ctx).SetSQL(sql)
}

// SetHTTPRequest sets the information of HTTP requests.
func (seg *Segment) SetHTTPRequest(request *schema.HTTPRequest) {
	if seg == nil {
		return
	}
	seg.mu.Lock()
	defer seg.mu.Unlock()
	if seg.http == nil {
		seg.http = &schema.HTTP{}
	}
	seg.http.Request = request
}

// SetHTTPRequest sets the information of HTTP requests.
func SetHTTPRequest(ctx context.Context, request *schema.HTTPRequest) {
	ContextSegment(ctx).SetHTTPRequest(request)
}

// SetHTTPResponse sets the information of HTTP requests.
func (seg *Segment) SetHTTPResponse(response *schema.HTTPResponse) {
	if seg == nil {
		return
	}
	seg.mu.Lock()
	defer seg.mu.Unlock()
	if seg.http == nil {
		seg.http = &schema.HTTP{}
	}
	seg.http.Response = response
}

// SetHTTPResponse sets the information of HTTP requests.
func SetHTTPResponse(ctx context.Context, response *schema.HTTPResponse) {
	ContextSegment(ctx).SetHTTPResponse(response)
}
