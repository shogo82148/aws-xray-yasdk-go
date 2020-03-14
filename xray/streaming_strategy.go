package xray

import (
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

// StreamingStrategy provides an interface for implementing streaming strategies.
type StreamingStrategy interface {
	StreamSegment(seg *Segment) []*schema.Segment
}

type streamingStrategyBatchAll struct{}

// NewStreamingStrategyBatchAll returns a streaming strategy.
func NewStreamingStrategyBatchAll() StreamingStrategy {
	return &streamingStrategyBatchAll{}
}

func (s *streamingStrategyBatchAll) StreamSegment(seg *Segment) []*schema.Segment {
	if !seg.isRoot() {
		return nil
	}
	seg.mu.Lock()
	defer seg.mu.Unlock()
	return []*schema.Segment{serialize(seg)}
}

func serialize(seg *Segment) *schema.Segment {
	originTime := seg.root.startTime
	originEpoch := float64(originTime.Unix()) + float64(originTime.Nanosecond())/1e9
	ret := &schema.Segment{
		Name:      seg.name,
		ID:        seg.id,
		StartTime: originEpoch + seg.startTime.Sub(originTime).Seconds(),

		Error:    seg.error,
		Throttle: seg.throttle,
		Fault:    seg.fault,
		Cause:    seg.cause,

		Namespace: seg.namespace,
		Metadata:  seg.metadata,
		SQL:       seg.sql,
		HTTP:      seg.http,
	}

	if seg.inProgress() {
		ret.InProgress = true
	} else {
		seg.status = segmentStatusEmitted
		seg.root.emittedSegments++

		// use monotonic clock instead of wall clock to get correct proccessing time.
		// https://golang.org/pkg/time/#hdr-Monotonic_Clocks
		ret.EndTime = originEpoch + seg.endTime.Sub(originTime).Seconds()
	}
	if seg.isRoot() {
		ret.TraceID = seg.traceID
		if parentID := seg.traceHeader.ParentID; parentID != "" {
			// the parent is on upstream
			ret.ParentID = parentID
			ret.Type = "subsegment"
		}
	}

	for _, sub := range seg.subsegments {
		sub.mu.Lock()
		ret.Subsegments = append(ret.Subsegments, serialize(sub))
		sub.mu.Unlock()
	}

	return ret
}

type streamingStrategyLimitSubsegment struct {
	limit int
}

// NewStreamingStrategyLimitSubsegment returns a streaming strategy.
func NewStreamingStrategyLimitSubsegment(limit int) StreamingStrategy {
	if limit < 0 {
		panic("xray: limit should not be negative")
	}
	return &streamingStrategyLimitSubsegment{
		limit: limit + 1,
	}
}

func serializeIndependentSubsegment(seg *Segment) *schema.Segment {
	originTime := seg.root.startTime
	originEpoch := float64(originTime.Unix()) + float64(originTime.Nanosecond())/1e9
	ret := &schema.Segment{
		Name:      seg.name,
		ID:        seg.id,
		TraceID:   seg.traceID,
		StartTime: originEpoch + seg.startTime.Sub(originTime).Seconds(),

		Error:    seg.error,
		Throttle: seg.throttle,
		Fault:    seg.fault,
		Cause:    seg.cause,

		Namespace: seg.namespace,
		Metadata:  seg.metadata,
		SQL:       seg.sql,
		HTTP:      seg.http,
	}

	if seg.inProgress() {
		ret.InProgress = true
	} else {
		// use monotonic clock instead of wall clock to get correct proccessing time.
		// https://golang.org/pkg/time/#hdr-Monotonic_Clocks
		ret.EndTime = originEpoch + seg.endTime.Sub(originTime).Seconds()
	}

	if seg.isRoot() {
		if parentID := seg.traceHeader.ParentID; parentID != "" {
			// the parent is on upstream
			ret.ParentID = parentID
			ret.Type = "subsegment"
		}
	} else {
		ret.ParentID = seg.parent.id
		ret.Type = "subsegment"
	}
	return ret
}

func (s *streamingStrategyLimitSubsegment) StreamSegment(seg *Segment) []*schema.Segment {
	root := seg.root
	root.mu.Lock()
	defer root.mu.Unlock()

	// fast pass for batching all subsegments.
	if root.totalSegments <= s.limit {
		if seg.isRoot() {
			return []*schema.Segment{serialize(seg)}
		}
		return nil
	}

	ctx := &streamingStrategyLimitSubsegmentContext{}
	ctx.serialize(root)
	return ctx.result
}

type streamingStrategyLimitSubsegmentContext struct {
	result []*schema.Segment
}

// search completed segments and emit if it is not emitted.
func (ctx *streamingStrategyLimitSubsegmentContext) serialize(seg *Segment) {
	if !seg.inProgress() && seg.status != segmentStatusEmitted {
		ctx.result = append(ctx.result, serializeIndependentSubsegment(seg))
		seg.status = segmentStatusEmitted
		seg.root.emittedSegments++
	}

	for _, sub := range seg.subsegments {
		sub.mu.Lock()
		ctx.serialize(sub)
		sub.mu.Unlock()
	}
}
