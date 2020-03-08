package xray

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

func TestCapture(t *testing.T) {
	ctx, td := NewTestDaemon()
	defer td.Close()

	ctx, seg := BeginSegment(ctx, "root")
	err := Capture(ctx, "capture", func(ctx context.Context) error {
		return nil
	})
	seg.Close()
	if err != nil {
		t.Fatal(err)
	}

	want := &schema.Segment{
		Name:    "root",
		ID:      seg.id,
		TraceID: seg.traceID,
		Subsegments: []*schema.Segment{
			{
				Name: "capture",
			},
		},
	}
	got, err := td.Recv()
	if err != nil {
		t.Error(err)
	}

	// ignore time
	got.StartTime = 0
	got.EndTime = 0
	got.Subsegments[0].ID = ""
	got.Subsegments[0].StartTime = 0
	got.Subsegments[0].EndTime = 0

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}
