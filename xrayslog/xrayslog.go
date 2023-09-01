//go:build go1.21
// +build go1.21

// Package xrayslog provides a [log/slog.Handler] that adds trace ID to the log record.
package xrayslog

import (
	"context"
	"log/slog"

	"github.com/shogo82148/aws-xray-yasdk-go/xray"
)

var _ slog.Handler = (*handler)(nil)

type handler struct {
	parent     slog.Handler
	traceIDKey string
	groups     []string
}

// Enable implements slog.Handler interface.
func (h *handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.parent.Enabled(ctx, level)
}

// Handle implements slog.Handler interface.
func (h *handler) Handle(ctx context.Context, record slog.Record) error {
	traceID := xray.ContextTraceID(ctx)
	if traceID == "" && len(h.groups) == 0 {
		// no trace ID and no groups. nothing to do.
		return h.parent.Handle(ctx, record)
	}

	var newRecord slog.Record
	if len(h.groups) == 0 {
		newRecord = record.Clone()
	} else {
		newRecord = slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
		attrs := make([]any, 0, record.NumAttrs())
		record.Attrs(func(a slog.Attr) bool {
			attrs = append(attrs, a)
			return true
		})
		for i := len(h.groups) - 1; i >= 0; i-- {
			attrs = []any{slog.Group(h.groups[i], attrs...)}
		}
		for _, attr := range attrs {
			newRecord.AddAttrs(attr.(slog.Attr))
		}
	}

	if traceID != "" {
		// add trace ID to the log record.
		newRecord.AddAttrs(slog.String(h.traceIDKey, traceID))
	}
	return h.parent.Handle(ctx, newRecord)
}

// WithAttrs implements slog.Handler interface.
func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &handler{
		parent:     h.parent.WithAttrs(attrs),
		traceIDKey: h.traceIDKey,
	}
}

// WithGroup implements slog.Handler interface.
func (h *handler) WithGroup(name string) slog.Handler {
	h2 := *h // shallow copy, but it is OK.
	h2.groups = append(h2.groups, name)
	return &h2
}

// NewHandler returns a slog.Handler that adds trace ID to the log record.
func NewHandler(parent slog.Handler, traceIDKey string) slog.Handler {
	return &handler{
		parent:     parent,
		traceIDKey: traceIDKey,
	}
}