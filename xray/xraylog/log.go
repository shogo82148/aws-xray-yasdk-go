// Package xraylog implements a logger with a log level, and an interface for a custom logger.
// By default, the SDK logs error messages to the os.Stderr.
// The log level of the built in logger can be set by using either
// the AWS_XRAY_DEBUG_MODE or AWS_XRAY_LOG_LEVEL environment variables.
// If AWS_XRAY_DEBUG_MODE is set, the log level is set to the debug level.
// AWS_XRAY_LOG_LEVEL may be set to debug, info, warn, error or silent.
// This value is ignored if AWS_XRAY_DEBUG_MODE is set.
package xraylog

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// contextKey is a value for use with context.WithValue. It's used as
// a pointer so it fits in an any without allocation.
type contextKey struct {
	name string
}

func (k *contextKey) String() string { return "xray context value " + k.name }

var loggerContextKey = &contextKey{"logger"}

var mu sync.RWMutex
var globalLogger Logger

func init() {
	level := LogLevelInfo
	if os.Getenv("AWS_XRAY_DEBUG_MODE") != "" {
		level = LogLevelDebug
	} else if env := os.Getenv("AWS_XRAY_LOG_LEVEL"); env != "" {
		env = strings.ToLower(env)
		switch env {
		case "debug":
			level = LogLevelDebug
		case "info":
			level = LogLevelInfo
		case "warn":
			level = LogLevelWarn
		case "error":
			level = LogLevelError
		case "silent":
			globalLogger = NullLogger{}
			return
		}
	}
	globalLogger = NewDefaultLogger(os.Stderr, level)
}

// Logger is the logging interface used by X-Ray YA-SDK.
type Logger interface {
	// Log outputs the msg into the log. msg is fmt.Stringer because of lazy evaluation.
	// It may be called concurrently from multiple goroutines.
	Log(ctx context.Context, level LogLevel, msg fmt.Stringer)
}

// LogLevel represents the severity of a log message, where a higher value
// means more severe. The integer value should not be serialized as it is
// subject to change.
type LogLevel int

const (
	// LogLevelDebug is debug level.
	LogLevelDebug LogLevel = iota + 1

	// LogLevelInfo is info level.
	LogLevelInfo

	// LogLevelWarn is warn level.
	LogLevelWarn

	// LogLevelError is error level.
	LogLevelError
)

func (ll LogLevel) String() string {
	switch ll {
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelWarn:
		return "WARN"
	case LogLevelError:
		return "ERROR"
	default:
		return fmt.Sprintf("UNKNOWNLOGLEVEL<%d>", ll)
	}
}

// SetLogger updates the global logger.
func SetLogger(logger Logger) {
	if logger == nil {
		panic("logger should not be nil")
	}
	mu.Lock()
	defer mu.Unlock()
	globalLogger = logger
}

// WithLogger set the context logger.
func WithLogger(ctx context.Context, logger Logger) context.Context {
	return context.WithValue(ctx, loggerContextKey, logger)
}

// ContextLogger returns the context logger.
// If the context has no logger, returns the global logger.
func ContextLogger(ctx context.Context) Logger {
	logger := ctx.Value(loggerContextKey)
	if logger != nil {
		return logger.(Logger)
	}
	mu.RLock()
	defer mu.RUnlock()
	return globalLogger
}

type defaultLogger struct {
	mu       sync.Mutex
	w        io.Writer
	minLevel LogLevel
	pool     sync.Pool
}

// NewDefaultLogger returns new logger that outputs into w.
func NewDefaultLogger(w io.Writer, minLevel LogLevel) Logger {
	return &defaultLogger{
		w:        w,
		minLevel: minLevel,
		pool: sync.Pool{
			New: func() any {
				return new(bytes.Buffer)
			},
		},
	}
}

func (l *defaultLogger) Log(ctx context.Context, level LogLevel, msg fmt.Stringer) {
	if level < l.minLevel {
		return
	}

	// evaluate the message text lazily
	str := msg.String()

	buf := l.pool.Get().(*bytes.Buffer)
	defer l.pool.Put(buf)
	buf.Reset()
	buf.WriteString(time.Now().Format(time.RFC3339))
	buf.WriteString(" [")
	buf.WriteString(level.String())
	buf.WriteString("] ")
	buf.WriteString(str)
	buf.WriteString("\n")

	l.mu.Lock()
	defer l.mu.Unlock()
	l.w.Write(buf.Bytes())
}

// NullLogger suppress all logs.
type NullLogger struct{}

// Log implements Logger.
func (NullLogger) Log(_ context.Context, _ LogLevel, _ fmt.Stringer) {
	// do nothing
}

// Info outputs info level log message.
func Info(ctx context.Context, v ...any) {
	ContextLogger(ctx).Log(ctx, LogLevelInfo, printArgs{args: v})
}

// Infof outputs info level log message.
func Infof(ctx context.Context, format string, v ...any) {
	ContextLogger(ctx).Log(ctx, LogLevelInfo, printfArgs{format: format, args: v})
}

// Debug outputs debug level log message.
func Debug(ctx context.Context, v ...any) {
	ContextLogger(ctx).Log(ctx, LogLevelDebug, printArgs{args: v})
}

// Debugf outputs debug level log message.
func Debugf(ctx context.Context, format string, v ...any) {
	ContextLogger(ctx).Log(ctx, LogLevelDebug, printfArgs{format: format, args: v})
}

// Warn outputs warn level log message.
func Warn(ctx context.Context, v ...any) {
	ContextLogger(ctx).Log(ctx, LogLevelWarn, printArgs{args: v})
}

// Warnf outputs warn level log message.
func Warnf(ctx context.Context, format string, v ...any) {
	ContextLogger(ctx).Log(ctx, LogLevelWarn, printfArgs{format: format, args: v})
}

// Error outputs error level log message.
func Error(ctx context.Context, v ...any) {
	ContextLogger(ctx).Log(ctx, LogLevelError, printArgs{args: v})
}

// Errorf outputs warn level log message.
func Errorf(ctx context.Context, format string, v ...any) {
	ContextLogger(ctx).Log(ctx, LogLevelError, printfArgs{format: format, args: v})
}

type printArgs struct {
	args []any
}

func (args printArgs) String() string {
	return fmt.Sprint(args.args...)
}

type printfArgs struct {
	format string
	args   []any
}

func (args printfArgs) String() string {
	return fmt.Sprintf(args.format, args.args...)
}
