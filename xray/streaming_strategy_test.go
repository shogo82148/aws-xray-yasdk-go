package xray

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

func TestStreamingStrategyBatchAll(t *testing.T) {
	now := time.Date(2001, time.September, 9, 1, 46, 40, 0, time.UTC)
	seg := &Segment{
		name:           "root segment",
		id:             "03babb4ba280be51",
		traceID:        "1-5e645f3e-1dfad076a177c5ccc5de12f5",
		startTime:      now,
		endTime:        now.Add(time.Second),
		totalSegments:  4,
		closedSegments: 4,
	}
	seg.root = seg
	child1 := &Segment{
		parent:    seg,
		root:      seg,
		name:      "child1",
		id:        "acc82ea453399569",
		traceID:   seg.traceID,
		startTime: now,
		endTime:   now.Add(time.Second),
	}
	child2 := &Segment{
		parent:    seg,
		root:      seg,
		name:      "child2",
		id:        "6c78818fe7682a62",
		traceID:   seg.traceID,
		startTime: now,
		endTime:   now.Add(time.Second),
	}
	grandchild := &Segment{
		parent:    child2,
		root:      seg,
		name:      "grandchild",
		id:        "bebb747c66f386a5",
		traceID:   seg.traceID,
		startTime: now,
		endTime:   now.Add(time.Second),
	}
	seg.subsegments = append(seg.subsegments, child1, child2)
	child2.subsegments = append(child2.subsegments, grandchild)

	strategy := NewStreamingStrategyBatchAll()

	if ret := strategy.StreamSegment(child1); len(ret) != 0 {
		t.Errorf("want len(ret) = 0, got %v", ret)
	}
	if ret := strategy.StreamSegment(child2); len(ret) != 0 {
		t.Errorf("want len(ret) = 0, got %v", ret)
	}
	if ret := strategy.StreamSegment(grandchild); len(ret) != 0 {
		t.Errorf("want len(ret) = 0, got %v", ret)
	}

	got := strategy.StreamSegment(seg)
	want := []*schema.Segment{
		{
			Name:      "root segment",
			ID:        "03babb4ba280be51",
			TraceID:   "1-5e645f3e-1dfad076a177c5ccc5de12f5",
			StartTime: 1000000000,
			EndTime:   1000000001,
			Subsegments: []*schema.Segment{
				{
					Name:      "child1",
					ID:        "acc82ea453399569",
					StartTime: 1000000000,
					EndTime:   1000000001,
				},
				{
					Name:      "child2",
					ID:        "6c78818fe7682a62",
					StartTime: 1000000000,
					EndTime:   1000000001,
					Subsegments: []*schema.Segment{
						{
							Name:      "grandchild",
							ID:        "bebb747c66f386a5",
							StartTime: 1000000000,
							EndTime:   1000000001,
						},
					},
				},
			},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("StreamSegment(seg) mismatch (-want +got):\n%s", diff)
	}
	if seg.emittedSegments != 4 {
		t.Errorf("want %d, got %d", 4, seg.emittedSegments)
	}
	if child1.status != segmentStatusEmitted {
		t.Errorf("want %d, got %d", segmentStatusEmitted, child1.status)
	}
	if child2.status != segmentStatusEmitted {
		t.Errorf("want %d, got %d", segmentStatusEmitted, child2.status)
	}
	if grandchild.status != segmentStatusEmitted {
		t.Errorf("want %d, got %d", segmentStatusEmitted, grandchild.status)
	}
}

func TestStreamingStrategyLimitSubsegment(t *testing.T) {
	now := time.Date(2001, time.September, 9, 1, 46, 40, 0, time.UTC)

	// create the segment that have one subsegment.
	newSegment := func() *Segment {
		seg := &Segment{
			name:           "root segment",
			id:             "03babb4ba280be51",
			traceID:        "1-5e645f3e-1dfad076a177c5ccc5de12f5",
			startTime:      now,
			totalSegments:  2,
			closedSegments: 1,
		}
		seg.root = seg
		child1 := &Segment{
			parent:    seg,
			root:      seg,
			name:      "child1",
			id:        "acc82ea453399569",
			traceID:   seg.traceID,
			startTime: now,
			endTime:   now.Add(time.Second),
		}
		seg.subsegments = append(seg.subsegments, child1)
		return seg
	}

	t.Run("limit 0", func(t *testing.T) {
		seg := newSegment()
		strategy := NewStreamingStrategyLimitSubsegment(0)
		got := strategy.StreamSegment(seg.subsegments[0])
		want := []*schema.Segment{
			{
				Name:      "child1",
				ID:        "acc82ea453399569",
				ParentID:  "03babb4ba280be51",
				Type:      "subsegment",
				TraceID:   "1-5e645f3e-1dfad076a177c5ccc5de12f5",
				StartTime: 1000000000,
				EndTime:   1000000001,
			},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("StreamSegment(seg) mismatch (-want +got):\n%s", diff)
		}
		if seg.emittedSegments != 1 {
			t.Errorf("want %d, got %d", 1, seg.emittedSegments)
		}
	})

	t.Run("limit 1", func(t *testing.T) {
		seg := newSegment()
		strategy := NewStreamingStrategyLimitSubsegment(1)
		if got := strategy.StreamSegment(seg.subsegments[0]); got != nil {
			t.Errorf("want nil, got %v", got)
		}
		got := strategy.StreamSegment(seg)
		want := []*schema.Segment{
			{
				Name:       "root segment",
				ID:         "03babb4ba280be51",
				TraceID:    "1-5e645f3e-1dfad076a177c5ccc5de12f5",
				StartTime:  1000000000,
				InProgress: true,
				Subsegments: []*schema.Segment{
					{
						Name:      "child1",
						ID:        "acc82ea453399569",
						StartTime: 1000000000,
						EndTime:   1000000001,
					},
				},
			},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("StreamSegment(seg) mismatch (-want +got):\n%s", diff)
		}
		if seg.emittedSegments != 1 {
			t.Errorf("want %d, got %d", 1, seg.emittedSegments)
		}
	})
}
