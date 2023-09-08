package ctxmissing

import (
	"context"
)

// IgnoreStrategy is a [Strategy] that do nothing when the segment context is missing.
type IgnoreStrategy struct{}

// ContextMissing implements [Strategy].
func (*IgnoreStrategy) ContextMissing(ctx context.Context, v any) {
	// do nothing
}

// NewIgnoreStrategy returns a new IgnoreStrategy.
func NewIgnoreStrategy() *IgnoreStrategy {
	return &IgnoreStrategy{}
}
