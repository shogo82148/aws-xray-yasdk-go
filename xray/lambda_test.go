package xray

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

func TestBeginSubsegment_ForLambda(t *testing.T) {
	nowFunc = fixedTime
	defer func() { nowFunc = time.Now }()

	ctx, td := NewTestDaemon(nil)
	defer td.Close()

	ctx = context.WithValue(ctx, lambdaContextKey, "Root=1-5e645f3e-1dfad076a177c5ccc5de12f5;Parent=03babb4ba280be51")
	ctx, seg := BeginSubsegment(ctx, "subsegment")
	_ = ctx // do something using ctx
	seg.Close()

	got, err := td.Recv()
	if err != nil {
		t.Error(err)
	}
	want := &schema.Segment{
		Name:      "subsegment",
		ID:        seg.id,
		TraceID:   "1-5e645f3e-1dfad076a177c5ccc5de12f5",
		StartTime: 1000000000,
		EndTime:   1000000000,
		ParentID:  "03babb4ba280be51",
		Type:      "subsegment",
		Service:   ServiceData,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}
