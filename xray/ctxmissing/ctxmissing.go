package ctxmissing

import "context"

// Strategy provides an interface for
// implementing context missing strategies.
type Strategy interface {
	ContextMissing(ctx context.Context, v any)
}
