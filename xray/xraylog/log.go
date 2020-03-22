package xraylog

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// contextKey is a value for use with context.WithValue. It's used as
// a pointer so it fits in an interface{} without allocation.
type contextKey struct {
	name string
}

func (k *contextKey) String() string { return "xray context value " + k.name }

var loggerContextKey = &contextKey{"logger"}

var globalLogger Logger

func init() {
	level := LogLevelInfo
	if os.Getenv("AWS_XRAY_DEBUG_MODE") != "" {
		level = LogLevelDebug
	}
	globalLogger = NewDefaultLogger(os.Stderr, level)
}

// Logger is the logging interface used by xray.
type Logger interface {
	Log(level LogLevel, msg string)
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
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
	}
}

func (l *defaultLogger) Log(level LogLevel, msg string) {
	if level < l.minLevel {
		return
	}

	buf := l.pool.Get().(*bytes.Buffer)
	defer l.pool.Put(buf)
	buf.Reset()
	buf.WriteString(time.Now().Format(time.RFC3339))
	buf.WriteString(" [")
	buf.WriteString(level.String())
	buf.WriteString("] ")
	buf.WriteString(msg)
	buf.WriteString("\n")

	l.mu.Lock()
	defer l.mu.Unlock()
	l.w.Write(buf.Bytes())
}

// Info outputs info level log message.
func Info(ctx context.Context, v ...interface{}) {
	ContextLogger(ctx).Log(LogLevelInfo, fmt.Sprint(v...))
}

// Infof outputs info level log message.
func Infof(ctx context.Context, format string, v ...interface{}) {
	ContextLogger(ctx).Log(LogLevelInfo, fmt.Sprintf(format, v...))
}

// Debug outputs debug level log message.
func Debug(ctx context.Context, v ...interface{}) {
	ContextLogger(ctx).Log(LogLevelDebug, fmt.Sprint(v...))
}

// Debugf outputs debug level log message.
func Debugf(ctx context.Context, format string, v ...interface{}) {
	ContextLogger(ctx).Log(LogLevelDebug, fmt.Sprintf(format, v...))
}

// Warn outputs warn level log message.
func Warn(ctx context.Context, v ...interface{}) {
	ContextLogger(ctx).Log(LogLevelWarn, fmt.Sprint(v...))
}

// Warnf outputs warn level log message.
func Warnf(ctx context.Context, format string, v ...interface{}) {
	ContextLogger(ctx).Log(LogLevelWarn, fmt.Sprintf(format, v...))
}

// Error outputs error level log message.
func Error(ctx context.Context, v ...interface{}) {
	ContextLogger(ctx).Log(LogLevelError, fmt.Sprint(v...))
}

// Errorf outputs warn level log message.
func Errorf(ctx context.Context, format string, v ...interface{}) {
	ContextLogger(ctx).Log(LogLevelError, fmt.Sprintf(format, v...))
}
