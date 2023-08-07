//go:build go1.21
// +build go1.21

package xray

import (
	"context"
)

// DetachContextSegment returns a new context with the existing segment.
// All values associated with ctx are also associated with the new context.
// This is useful for creating background tasks which won't be cancelled
// when a request completes.
//
// On Go 1.21 or later, this function is just an alias of context.WithoutCancel.
// Use context.WithoutCancel directly.
func DetachContextSegment(ctx context.Context) context.Context {
	return context.WithoutCancel(ctx)
}
