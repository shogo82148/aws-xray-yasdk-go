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

	//lint:ignore SA1029 lambdaContextKey should be string because of compatibility with AWS Lambda for Go
	// ref. https://github.com/aws/aws-lambda-go/blob/14da40f6fad9d5629abe069408b8ec278c36db75/lambda/function.go#L61
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
		AWS:       xrayData,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestBeginSubsegment_ForLambda_Nested(t *testing.T) {
	nowFunc = fixedTime
	defer func() { nowFunc = time.Now }()

	ctx, td := NewTestDaemon(nil)
	defer td.Close()

	//lint:ignore SA1029 lambdaContextKey should be string because of compatibility with AWS Lambda for Go
	// ref. https://github.com/aws/aws-lambda-go/blob/14da40f6fad9d5629abe069408b8ec278c36db75/lambda/function.go#L61
	ctx = context.WithValue(ctx, lambdaContextKey, "Root=1-5e645f3e-1dfad076a177c5ccc5de12f5;Parent=03babb4ba280be51")
	ctx, seg0 := BeginSubsegment(ctx, "subsegment")
	ctx, seg1 := BeginSubsegment(ctx, "sub-sub-segment")
	_ = ctx // do something using ctx

	// the order of closing is upside‐down, but is OK.
	// ref. https://github.com/shogo82148/aws-xray-yasdk-go/pull/160
	// the seg1 is traced as a segment in progress.
	seg0.Close()

	// the seg1 is finished and traced
	seg1.Close()

	got0, err := td.Recv()
	if err != nil {
		t.Error(err)
	}
	want0 := &schema.Segment{
		Name:      "subsegment",
		ID:        seg0.id,
		TraceID:   "1-5e645f3e-1dfad076a177c5ccc5de12f5",
		StartTime: 1000000000,
		EndTime:   1000000000,
		ParentID:  "03babb4ba280be51",
		Type:      "subsegment",
		Service:   ServiceData,
		AWS:       xrayData,
		Subsegments: []*schema.Segment{
			{
				Name:       "sub-sub-segment",
				ID:         seg1.id,
				StartTime:  1000000000,
				InProgress: true,
			},
		},
	}
	if diff := cmp.Diff(want0, got0); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}

	got1, err := td.Recv()
	if err != nil {
		t.Error(err)
	}
	want1 := &schema.Segment{
		Name:      "sub-sub-segment",
		ID:        seg1.id,
		TraceID:   "1-5e645f3e-1dfad076a177c5ccc5de12f5",
		StartTime: 1000000000,
		EndTime:   1000000000,
		ParentID:  seg0.id,
		Type:      "subsegment",
	}
	if diff := cmp.Diff(want1, got1); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}
