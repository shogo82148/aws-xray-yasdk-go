package xray

import (
	"context"
	"testing"
)

func TestClientEmit(t *testing.T) {
	c := New("127.0.0.1:2000")
	c.Emit(context.Background(), nil)
}
