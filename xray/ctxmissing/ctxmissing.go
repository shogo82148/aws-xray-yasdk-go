// Package ctxmissing provides the context missing strategy.
package ctxmissing

import "context"

// Strategy provides an interface for
// implementing context missing strategies.
type Strategy interface {
	// ContextMissing is called when any segment is not associated with a context.
	ContextMissing(ctx context.Context, v any)
}
