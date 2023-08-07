package xray

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

func TestDetachContextSegment(t *testing.T) {
	nowFunc = fixedTime
	defer func() { nowFunc = time.Now }()

	ctx, td := NewTestDaemon(nil)
	defer td.Close()

	ctx, cancel := context.WithCancel(ctx)
	ctx1, _ := BeginSegment(ctx, "foobar")
	ctx2 := DetachContextSegment(ctx1)
	cancel() // ctx1 is canceled.

	seg := ContextSegment(ctx2) // get segment from ctx2
	seg.Close()

	want := &schema.Segment{
		Name:      "foobar",
		ID:        seg.id,
		TraceID:   seg.traceID,
		StartTime: 1000000000,
		EndTime:   1000000000,
		Service:   ServiceData,
		AWS:       xrayData,
	}
	got, err := td.Recv()
	if err != nil {
		t.Error(err)
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}

	select {
	case <-ctx2.Done():
		t.Error(ctx2.Err())
	default:
		// ctx1 is canceled, but ctx2 is not.
	}
}
