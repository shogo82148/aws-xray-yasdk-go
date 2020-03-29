package xray

import (
	"testing"

	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

func BenchmarkClient(b *testing.B) {
	ctx, td := NewNullDaemon()
	defer td.Close()
	client := ContextClient(ctx)
	seg := &schema.Segment{
		Name:      "foobar",
		ID:        "03babb4ba280be51",
		TraceID:   "1-5e645f3e-1dfad076a177c5ccc5de12f5",
		StartTime: 1000000000,
		EndTime:   1000000000,
		Service:   ServiceData,
		AWS:       xrayData,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client.emit(ctx, seg)
	}
}
