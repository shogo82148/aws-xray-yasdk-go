package xraylog

import (
	"bytes"
	"context"
	"io"
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

func BenchmarkLogError(b *testing.B) {
	logger := NewDefaultLogger(io.Discard, LogLevelWarn)
	ctx := WithLogger(context.Background(), logger)
	for i := 0; i < b.N; i++ {
		Errorf(ctx, "something wrong: %v", "foobar")
	}
}

func BenchmarkLogDebug(b *testing.B) {
	logger := NewDefaultLogger(io.Discard, LogLevelWarn)
	ctx := WithLogger(context.Background(), logger)
	for i := 0; i < b.N; i++ {
		Debugf(ctx, "something wrong: %v", "foobar")
	}
}
