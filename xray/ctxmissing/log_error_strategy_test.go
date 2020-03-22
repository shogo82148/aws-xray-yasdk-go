package ctxmissing

import (
	"bytes"
	"context"
	"testing"

	"github.com/shogo82148/aws-xray-yasdk-go/xray/xraylog"
)

var _ Strategy = (*LogErrorStrategy)(nil)

func TestLogError(t *testing.T) {
	var buf bytes.Buffer
	logger := xraylog.NewDefaultLogger(&buf, xraylog.LogLevelError)
	ctx := xraylog.WithLogger(context.Background(), logger)

	strategy := &LogErrorStrategy{}
	strategy.ContextMissing(ctx, "MISSING!!!")

	if !bytes.Contains(buf.Bytes(), []byte("MISSING!!!")) {
		t.Errorf("unexpected log: %s", buf.String())
	}
}
