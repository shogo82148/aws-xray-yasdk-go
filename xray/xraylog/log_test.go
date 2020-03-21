package xraylog

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := NewDefaultLogger(&buf, LogLevelWarn)
	ctx := WithLogger(context.Background(), logger)

	Debug(ctx, "debug")
	Info(ctx, "info")
	Warn(ctx, "warn")
	Error(ctx, "error")

	lines := strings.Split(buf.String(), "\n")
	if !strings.HasSuffix(lines[0], " [WARN] warn") {
		t.Errorf("expected first line to be warn, got %q", lines[0])
	}
	if !strings.HasSuffix(lines[1], " [ERROR] error") {
		t.Errorf("expected first line to be error, got %q", lines[1])
	}
}
