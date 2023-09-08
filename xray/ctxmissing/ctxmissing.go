// Package ctxmissing provides the context missing strategy.
package ctxmissing

import "context"

// Strategy provides an interface for
// implementing context missing strategies.
type Strategy interface {
	// ContextMissing is called when any segment is not associated with a context.
	ContextMissing(ctx context.Context, v any)
}

var _ Strategy = StrategyFunc(nil)

// StrategyFunc is an adapter to allow the use of
// ordinary functions as context missing strategies.
type StrategyFunc func(ctx context.Context, v any)

func (s StrategyFunc) ContextMissing(ctx context.Context, v any) {
	s(ctx, v)
}
