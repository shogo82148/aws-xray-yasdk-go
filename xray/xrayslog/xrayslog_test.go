//go:build go1.21
// +build go1.21

package xrayslog

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/xraylog"
)

func Test_getLogLevelFromEnv(t *testing.T) {
	t.Run("AWS_XRAY_DEBUG_MODE is enable", func(t *testing.T) {
		t.Setenv("AWS_XRAY_DEBUG_MODE", "1")
		t.Setenv("AWS_XRAY_LOG_LEVEL", "info")
		got := getLogLevelFromEnv()
		want := xraylog.LogLevelDebug
		if got != want {
			t.Errorf("got %v; want %v", got, want)
		}
	})

	tests := []struct {
		env  string
		want xraylog.LogLevel
	}{
		{"debug", xraylog.LogLevelDebug},
		{"info", xraylog.LogLevelInfo},
		{"warn", xraylog.LogLevelWarn},
		{"error", xraylog.LogLevelError},
		{"silent", xraylog.LogLevelSilent},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.env, func(t *testing.T) {
			t.Setenv("AWS_XRAY_DEBUG_MODE", "")
			t.Setenv("AWS_XRAY_LOG_LEVEL", tt.env)
			got := getLogLevelFromEnv()
			want := tt.want
			if got != want {
				t.Errorf("got %v; want %v", got, want)
			}
		})
	}
}

func TestHandle_WithoutTraceID(t *testing.T) {
	// build the logger
	w := &bytes.Buffer{}
	parent := slog.NewJSONHandler(w, nil)
	h := NewHandler(parent, "trace_id")
	logger := slog.New(h)

	ctx := context.Background()

	// test the logger
	logger.InfoContext(ctx, "hello")
	var v map[string]any
	if err := json.Unmarshal(w.Bytes(), &v); err != nil {
		t.Error(err)
	}
	if got, ok := v["trace_id"]; ok {
		t.Errorf("trace_id should be empty, but got %s", got)
	}
}

func TestHandle_WithTraceID(t *testing.T) {
	// build the logger
	w := &bytes.Buffer{}
	parent := slog.NewJSONHandler(w, nil)
	h := NewHandler(parent, "trace_id")
	logger := slog.New(h)

	// begin a new segment
	ctx := context.Background()
	ctx, segment := xray.BeginSegment(ctx, "my-segment")
	defer segment.Close()

	// test the logger
	logger.InfoContext(ctx, "hello")
	var v map[string]any
	if err := json.Unmarshal(w.Bytes(), &v); err != nil {
		t.Error(err)
	}
	if v["trace_id"] != xray.ContextTraceID(ctx) {
		t.Errorf("trace_id is not set: %s", w.String())
	}
}

func TestWithAttrs(t *testing.T) {
	// build the logger
	w := &bytes.Buffer{}
	parent := slog.NewJSONHandler(w, nil)
	h := NewHandler(parent, "trace_id")
	logger := slog.New(h.WithAttrs([]slog.Attr{
		slog.String("foo", "bar"),
	}))

	// begin a new segment
	ctx := context.Background()
	ctx, segment := xray.BeginSegment(ctx, "my-segment")
	defer segment.Close()

	// test the logger
	logger.InfoContext(ctx, "hello")
	var v map[string]any
	if err := json.Unmarshal(w.Bytes(), &v); err != nil {
		t.Error(err)
	}
	if v["trace_id"] != xray.ContextTraceID(ctx) {
		t.Errorf("trace_id is not set: %s", w.String())
	}
	if v["foo"] != "bar" {
		t.Errorf("foo is not set: %s", w.String())
	}
}

func TestWithGroup_WithoutTraceID(t *testing.T) {
	// build the logger
	w := &bytes.Buffer{}
	parent := slog.NewJSONHandler(w, nil)
	h := NewHandler(parent, "trace_id")
	logger := slog.New(h.WithGroup("my-group1").WithGroup("my-group2"))

	ctx := context.Background()

	// test the logger
	logger.InfoContext(ctx, "hello", slog.String("foo", "bar"))
	var v map[string]any
	if err := json.Unmarshal(w.Bytes(), &v); err != nil {
		t.Error(err)
	}
	group, _ := v["my-group1"].(map[string]any)
	group, _ = group["my-group2"].(map[string]any)
	if group == nil || group["foo"] != "bar" {
		t.Errorf("foo is not set: %s", w.String())
	}
}

func TestWithGroup_WithTraceID(t *testing.T) {
	// build the logger
	w := &bytes.Buffer{}
	parent := slog.NewJSONHandler(w, nil)
	h := NewHandler(parent, "trace_id")
	logger := slog.New(h.WithGroup("my-group1").WithGroup("my-group2"))

	// begin a new segment
	ctx := context.Background()
	ctx, segment := xray.BeginSegment(ctx, "my-segment")
	defer segment.Close()

	// test the logger
	logger.InfoContext(ctx, "hello", slog.String("foo", "bar"))
	var v map[string]any
	if err := json.Unmarshal(w.Bytes(), &v); err != nil {
		t.Error(err)
	}
	if v["trace_id"] != xray.ContextTraceID(ctx) {
		t.Errorf("trace_id is not set: %s", w.String())
	}
	group, _ := v["my-group1"].(map[string]any)
	group, _ = group["my-group2"].(map[string]any)
	if group == nil || group["foo"] != "bar" {
		t.Errorf("foo is not set: %s", w.String())
	}
}

func TestNewXRayLogger(t *testing.T) {
	// build the logger
	w := &bytes.Buffer{}
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{
		AddSource: true,
	})

	// log
	ctx := context.Background()
	logger := NewXRayLoggerWithMinLevel(h, xraylog.LogLevelInfo)
	ctx = xraylog.WithLogger(ctx, logger)
	xraylog.Info(ctx, "Hello, World!")
	xraylog.Debug(ctx, "Hello, It's debug log and should be ignored")

	// check the result
	var v struct {
		Msg    string
		Level  string
		Source struct {
			Function string
			File     string
		}
	}
	if err := json.Unmarshal(w.Bytes(), &v); err != nil {
		t.Error(err)
	}
	if v.Msg != "Hello, World!" {
		t.Errorf("unexpected message: %s", v.Msg)
	}
	if v.Level != "INFO" {
		t.Errorf("unexpected level: %s", v.Level)
	}
	if filepath.Base(v.Source.File) != "xrayslog_test.go" {
		t.Errorf("unexpected source file: %s", v.Source.File)
	}
	if v.Source.Function != "github.com/shogo82148/aws-xray-yasdk-go/xray/xrayslog.TestNewXRayLogger" {
		t.Errorf("unexpected source function: %s", v.Source.Function)
	}
}
