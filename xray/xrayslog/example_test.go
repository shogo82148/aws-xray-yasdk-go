//go:build go1.21
// +build go1.21

package xrayslog_test

import (
	"context"
	"log/slog"
	"os"

	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/xrayslog"
)

func ExampleNewHandler() {
	// it's for testing.
	replace := func(groups []string, a slog.Attr) slog.Attr {
		// Remove time.
		if a.Key == slog.TimeKey && len(groups) == 0 {
			return slog.Attr{}
		}
		return a
	}

	// build the logger
	parent := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{ReplaceAttr: replace})
	h := xrayslog.NewHandler(parent, "trace_id")
	logger := slog.New(h)

	// begin a new segment
	ctx := context.Background()
	ctx, segment := xray.BeginSegmentWithHeader(ctx, "my-segment", "Root=1-5e645f3e-1dfad076a177c5ccc5de12f5")
	defer segment.Close()

	// output the log
	logger.InfoContext(ctx, "hello")

	// Output:
	// level=INFO msg=hello trace_id=1-5e645f3e-1dfad076a177c5ccc5de12f5
}
