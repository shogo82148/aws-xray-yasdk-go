//go:build go1.21
// +build go1.21

// Package xrayslog provides utilities for interfacing with the slog package.
package xrayslog

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/xraylog"
)

func getLogLevelFromEnv() xraylog.LogLevel {
	level := xraylog.LogLevelInfo
	if os.Getenv("AWS_XRAY_DEBUG_MODE") != "" {
		level = xraylog.LogLevelDebug
	} else if env := os.Getenv("AWS_XRAY_LOG_LEVEL"); env != "" {
		env = strings.ToLower(env)
		switch env {
		case "debug":
			level = xraylog.LogLevelDebug
		case "info":
			level = xraylog.LogLevelInfo
		case "warn":
			level = xraylog.LogLevelWarn
		case "error":
			level = xraylog.LogLevelError
		case "silent":
			return xraylog.LogLevelSilent
		}
	}
	return level
}

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

// NewHandler returns a [slog.Handler] that adds trace ID to the log record.
func NewHandler(parent slog.Handler, traceIDKey string) slog.Handler {
	return &handler{
		parent:     parent,
		traceIDKey: traceIDKey,
	}
}

type xrayLogger struct {
	h        slog.Handler
	minLevel xraylog.LogLevel
}

// NewXRayLogger returns a new [xraylog.Logger] such that each call to its Output method dispatches a Record to the specified handler.
// The logger acts as a bridge from the older xraylog API to newer structured logging handlers.
// The log level can be set by using either the AWS_XRAY_DEBUG_MODE or AWS_XRAY_LOG_LEVEL environment variables.
// If AWS_XRAY_DEBUG_MODE is set, the log level is set to the debug level.
// AWS_XRAY_LOG_LEVEL may be set to debug, info, warn, error or silent.
// This value is ignored if AWS_XRAY_DEBUG_MODE is set.
func NewXRayLogger(h slog.Handler) xraylog.Logger {
	return NewXRayLoggerWithMinLevel(h, getLogLevelFromEnv())
}

// NewXRayLoggerWithMinLevel returns a new [xraylog.Logger] such that each call to its Output method dispatches a Record to the specified handler.
// The logger acts as a bridge from the older xraylog API to newer structured logging handlers.
func NewXRayLoggerWithMinLevel(h slog.Handler, minLogLevel xraylog.LogLevel) xraylog.Logger {
	if minLogLevel == xraylog.LogLevelSilent {
		return xraylog.NullLogger{}
	}
	return &xrayLogger{h, minLogLevel}
}

func (l *xrayLogger) Log(ctx context.Context, level xraylog.LogLevel, msg fmt.Stringer) {
	if level < l.minLevel {
		return
	}

	lv := xraylogLevelToSlog(level)
	if !l.h.Enabled(ctx, lv) {
		return
	}

	// skip [runtime.Callers, l.Log, xraylog.Info]
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:])

	record := slog.NewRecord(time.Now(), lv, msg.String(), pcs[0])
	l.h.Handle(ctx, record)
}

func xraylogLevelToSlog(l xraylog.LogLevel) slog.Level {
	switch l {
	case xraylog.LogLevelDebug:
		return slog.LevelDebug
	case xraylog.LogLevelInfo:
		return slog.LevelInfo
	case xraylog.LogLevelWarn:
		return slog.LevelWarn
	case xraylog.LogLevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
