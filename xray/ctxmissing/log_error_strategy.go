package ctxmissing

import (
	"context"

	"github.com/shogo82148/aws-xray-yasdk-go/xray/xraylog"
)

// LogErrorStrategy is a [Strategy] that logs the error when the segment context is missing.
type LogErrorStrategy struct{}

// ContextMissing implements [Strategy].
func (*LogErrorStrategy) ContextMissing(ctx context.Context, v any) {
	xraylog.Errorf(ctx, "AWS X-Ray context missing: %v", v)
}

// NewLogErrorStrategy returns a new LogErrorStrategy.
func NewLogErrorStrategy() *LogErrorStrategy {
	return &LogErrorStrategy{}
}
