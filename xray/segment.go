package xray

import (
	"context"
	"crypto/rand"
	"fmt"
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

// Close closes the segment.
func (seg *Segment) Close() {
	if seg.parent != nil {
		Debugf(seg.ctx, "Closing subsegment named %s", seg.name)
	} else {
		Debugf(seg.ctx, "Closing segment named %s", seg.name)
	}
	seg.close()
	seg.emit()
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
	seg := ContextSegment(ctx)
	if seg == nil {
		return err != nil
	}
	return seg.AddError(err)
}

// SetNamespace sets namespace
func (seg *Segment) SetNamespace(namespace string) {
	seg.mu.Lock()
	defer seg.mu.Unlock()
	seg.namespace = namespace
}
