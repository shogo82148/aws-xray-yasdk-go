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
		name:      "root segment",
		id:        "03babb4ba280be51",
		traceID:   "1-5e645f3e-1dfad076a177c5ccc5de12f5",
		startTime: now,
		endTime:   now.Add(time.Second),
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
}
