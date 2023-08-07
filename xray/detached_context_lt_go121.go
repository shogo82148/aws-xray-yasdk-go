//go:build !go1.21
// +build !go1.21

package xray

import (
	"context"
	"reflect"
	"time"
)

// DetachContextSegment returns a new context with the existing segment.
// All values associated with ctx are also associated with the new context.
// This is useful for creating background tasks which won't be cancelled
// when a request completes.
func DetachContextSegment(ctx context.Context) context.Context {
	return &detachedContext{Context: ctx}
}

type detachedContext struct {
	context.Context
}

func (*detachedContext) Deadline() (deadline time.Time, ok bool) {
	return
}

func (*detachedContext) Done() <-chan struct{} {
	return nil
}

func (*detachedContext) Err() error {
	return nil
}

func (ctx *detachedContext) String() string {
	return contextName(ctx.Context) + ".Detached"
}

func contextName(c context.Context) string {
	type stringer interface {
		String() string
	}
	if s, ok := c.(stringer); ok {
		return s.String()
	}
	return reflect.TypeOf(c).String()
}
