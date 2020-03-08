package xray

import (
	"time"

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

	startTime := seg.startTime
	startEpoch := float64(startTime.Unix()) + float64(startTime.Nanosecond())/1e9
	ret := &schema.Segment{
		Name:      seg.name,
		ID:        seg.id,
		TraceID:   seg.traceID,
		StartTime: startEpoch,

		Error:    seg.error,
		Throttle: seg.throttle,
		Fault:    seg.fault,
		Cause:    seg.cause,
	}

	if seg.inProgress() {
		ret.InProgress = true
	} else {
		// use monotonic clock instead of wall clock to get correct proccessing time.
		// https://golang.org/pkg/time/#hdr-Monotonic_Clocks
		ret.EndTime = startEpoch + seg.endTime.Sub(startTime).Seconds()
	}

	for _, sub := range seg.subsegments {
		ret.Subsegments = append(ret.Subsegments, s.serializeSubsegment(startTime, startEpoch, sub))
	}
	seg.subsegments = nil

	return []*schema.Segment{ret}
}

func (s *streamingStrategyBatchAll) serializeSubsegment(startTime time.Time, startEpoch float64, seg *Segment) *schema.Segment {
	seg.mu.Lock()
	defer seg.mu.Unlock()

	ret := &schema.Segment{
		Name:      seg.name,
		ID:        seg.id,
		StartTime: startEpoch + seg.startTime.Sub(startTime).Seconds(),

		// trace id is not needed in embedded subsegments
		// TraceID: seg.traceID,

		Error:    seg.error,
		Throttle: seg.throttle,
		Fault:    seg.fault,
		Cause:    seg.cause,
	}

	if seg.inProgress() {
		ret.InProgress = true
	} else {
		ret.EndTime = startEpoch + seg.endTime.Sub(startTime).Seconds()
	}

	for _, sub := range seg.subsegments {
		ret.Subsegments = append(ret.Subsegments, s.serializeSubsegment(startTime, startEpoch, sub))
	}
	seg.subsegments = nil

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
		limit: limit,
	}
}

func (s *streamingStrategyLimitSubsegment) StreamSegment(seg *Segment) []*schema.Segment {
	seg.mu.Lock()
	defer seg.mu.Unlock()

	startTime := seg.startTime
	startEpoch := float64(startTime.Unix()) + float64(startTime.Nanosecond())/1e9
	ret := &schema.Segment{
		Name:      seg.name,
		ID:        seg.id,
		TraceID:   seg.traceID,
		StartTime: startEpoch,

		Error:    seg.error,
		Throttle: seg.throttle,
		Fault:    seg.fault,
		Cause:    seg.cause,
	}

	if seg.inProgress() {
		ret.InProgress = true
	} else {
		// use monotonic clock instead of wall clock to get correct proccessing time.
		// https://golang.org/pkg/time/#hdr-Monotonic_Clocks
		ret.EndTime = startEpoch + seg.endTime.Sub(startTime).Seconds()
	}

	if seg.parent != nil {
		ret.ParentID = seg.id
	}

	return []*schema.Segment{ret}
}
