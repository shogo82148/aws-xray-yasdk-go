package ctxmissing

import (
	"context"
	"testing"
)

var _ Strategy = (*RuntimeErrorStrategy)(nil)

func TestRuntimeError(t *testing.T) {
	var v any
	func() {
		defer func() {
			v = recover()
		}()
		strategy := &RuntimeErrorStrategy{}
		strategy.ContextMissing(context.Background(), "MISSING!!!")
	}()

	if s, ok := v.(string); !ok || s != "MISSING!!!" {
		t.Errorf("unexpected value: %v", v)
	}
}
