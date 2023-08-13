//go:build go1.21
// +build go1.21

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
}

// Enable implements slog.Handler interface.
func (h *handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.parent.Enabled(ctx, level)
}

// Handle implements slog.Handler interface.
func (h *handler) Handle(ctx context.Context, record slog.Record) error {
	traceID := xray.ContextTraceID(ctx)
	if traceID == "" {
		// there is no trace ID in the context.
		// don't add trace ID to the log record.
		return h.parent.Handle(ctx, record)
	}

	// add trace ID to the log record.
	newRecord := record.Clone()
	newRecord.AddAttrs(slog.String(h.traceIDKey, traceID))
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
	return &handler{
		parent:     h.parent.WithGroup(name),
		traceIDKey: h.traceIDKey,
	}
}

// NewHandler returns a slog.Handler that adds trace ID to the log record.
func NewHandler(parent slog.Handler, traceIDKey string) slog.Handler {
	return &handler{
		parent:     parent,
		traceIDKey: traceIDKey,
	}
}
