//go:build go1.21
// +build go1.21

package xrayslog

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/shogo82148/aws-xray-yasdk-go/xray"
)

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
