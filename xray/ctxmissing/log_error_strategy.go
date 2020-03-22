package ctxmissing

import (
	"context"

	"github.com/shogo82148/aws-xray-yasdk-go/xray/xraylog"
)

// LogErrorStrategy logs the error when the segment context is missing.
type LogErrorStrategy struct{}

// ContextMissing implements Strategy.
func (*LogErrorStrategy) ContextMissing(ctx context.Context, v interface{}) {
	xraylog.Errorf(ctx, "AWS X-Ray context missing: %v", v)
}
