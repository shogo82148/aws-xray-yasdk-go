package ctxmissing

import "context"

// RuntimeErrorStrategy panics when the segment context is missing.
type RuntimeErrorStrategy struct{}

// ContextMissing implements Strategy.
func (*RuntimeErrorStrategy) ContextMissing(ctx context.Context, v any) {
	panic(v)
}
