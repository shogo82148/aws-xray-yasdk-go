package xray

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
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
	loggerContextKey  = &contextKey{"logger"}
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

	// parent segment
	// if the segment is the root, the parent is nil.
	parent *Segment

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
	return ctx.Value(segmentContextKey).(*Segment)
}

// WithSegment returns a new context with the existing segment.
func WithSegment(ctx context.Context, seg *Segment) context.Context {
	return context.WithValue(ctx, segmentContextKey, seg)
}

// BeginSegment creates a new Segment for a given name and context.
//
// Caller should close the segment when the work is done.
func BeginSegment(ctx context.Context, name string) (context.Context, *Segment) {
	now := nowFunc()
	seg := &Segment{
		ctx:           ctx,
		name:          name, // TODO: @shogo82148 sanitize the name
		id:            NewSegmentID(),
		traceID:       NewTraceID(),
		startTime:     now,
		totalSegments: 1,
	}
	seg.root = seg
	ctx = context.WithValue(ctx, segmentContextKey, seg)
	return ctx, seg
}

// NewSegmentFromHeader creates a segment for downstream call and add information to the segment that gets from HTTP header.
func NewSegmentFromHeader(ctx context.Context, name string, r *http.Request, h TraceHeader) (context.Context, *Segment) {
	// TODO: set ParentID
	// TODO: sampling
	traceID := h.TraceID
	if traceID == "" {
		traceID = NewTraceID()
	}
	now := nowFunc()
	seg := &Segment{
		ctx:           ctx,
		name:          name, // TODO: @shogo82148 sanitize the name
		id:            NewSegmentID(),
		traceID:       traceID,
		startTime:     now,
		totalSegments: 1,
	}
	seg.root = seg
	ctx = context.WithValue(ctx, segmentContextKey, seg)
	return ctx, seg
}

// BeginSubsegment creates a new Segment for a given name and context.
//
// Caller should close the segment when the work is done.
func BeginSubsegment(ctx context.Context, name string) (context.Context, *Segment) {
	now := nowFunc()
	parent := ctx.Value(segmentContextKey).(*Segment)
	if parent == nil {
		panic("CONTEXT MISSING!") // TODO: see AWS_XRAY_CONTEXT_MISSING
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

type errorPanic struct {
	err interface{}
}

func (err *errorPanic) Error() string {
	return fmt.Sprintf("%T: %v", err.err, err.err)
}

// Close closes the segment.
func (seg *Segment) Close() {
	if seg.parent != nil {
		Debugf(seg.ctx, "Closing subsegment named %s", seg.name)
	} else {
		Debugf(seg.ctx, "Closing segment named %s", seg.name)
	}
	err := recover()
	if err != nil {
		seg.AddError(&errorPanic{err: err})
	}
	seg.close()
	seg.emit()
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
	client := seg.ctx.Value(clientContextKey)
	if client == nil {
		client = defaultClient
	}
	client.(*Client).Emit(seg.ctx, seg)
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

// SetNamespace sets namespace
func (seg *Segment) SetNamespace(namespace string) {
	seg.mu.Lock()
	defer seg.mu.Unlock()
	seg.namespace = namespace
}

// AddMetadata adds metadata.
func (seg *Segment) AddMetadata(key string, value interface{}) {
	if seg == nil {
		return
	}
	seg.mu.Lock()
	defer seg.mu.Unlock()
	if seg.metadata == nil {
		seg.metadata = map[string]interface{}{}
	}
	seg.metadata[key] = value
}

// AddMetadata adds metadata.
func AddMetadata(ctx context.Context, key string, value interface{}) {
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

func (seg *Segment) SetHTTP(http *schema.HTTP) {
	if seg == nil {
		return
	}
	seg.mu.Lock()
	defer seg.mu.Unlock()
	seg.http = http
}
