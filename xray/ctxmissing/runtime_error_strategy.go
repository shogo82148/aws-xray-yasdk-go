package ctxmissing

import "context"

// RuntimeErrorStrategy is a [Strategy] that panics when the segment context is missing.
type RuntimeErrorStrategy struct{}

// ContextMissing implements [Strategy].
func (*RuntimeErrorStrategy) ContextMissing(ctx context.Context, v any) {
	panic(v)
}

// NewRuntimeErrorStrategy returns a new RuntimeErrorStrategy.
func NewRuntimeErrorStrategy() *RuntimeErrorStrategy {
	return &RuntimeErrorStrategy{}
}
