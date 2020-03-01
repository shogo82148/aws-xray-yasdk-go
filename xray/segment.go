package xray

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

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

// Segment is a segment.
type Segment struct {
	mu        sync.RWMutex
	ctx       context.Context
	name      string
	id        string
	traceID   string
	startTime time.Time
	endTime   time.Time
}

// NewTraceID generates a string format of random trace ID.
func NewTraceID() string {
	var r [12]byte
	_, err := rand.Read(r[:])
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("1-%08x-%x", time.Now().Unix(), r)
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

// BeginSegment creates a new Segment for a given name and context.
//
// Caller should close the segment when the work is done.
func BeginSegment(ctx context.Context, name string) (context.Context, *Segment) {
	now := time.Now()
	seg := &Segment{
		ctx:       ctx,
		name:      name, // TODO: @shogo82148 sanitize the name
		id:        NewSegmentID(),
		traceID:   NewTraceID(),
		startTime: now,
	}
	ctx = context.WithValue(ctx, segmentContextKey, seg)
	return ctx, seg
}

// Close closes the segment.
func (seg *Segment) Close() {
	now := time.Now()
	seg.mu.Lock()
	seg.endTime = now
	seg.mu.Unlock()

	client := seg.ctx.Value(clientContextKey)
	if client == nil {
		client = defaultClient
	}
	client.(*Client).Emit(seg.ctx, seg)
}

func (seg *Segment) serialize() *schema.Segment {
	seg.mu.RLock()
	defer seg.mu.RUnlock()

	start := float64(seg.startTime.Unix()) + float64(seg.startTime.Nanosecond())/1e9
	end := start + seg.endTime.Sub(seg.startTime).Seconds()

	return &schema.Segment{
		Name:      seg.name,
		ID:        seg.id,
		TraceID:   seg.traceID,
		StartTime: start,
		EndTime:   end,
	}
}
