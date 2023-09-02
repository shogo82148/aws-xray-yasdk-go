//go:build go1.20
// +build go1.20

package xrayhttp

import (
	"net/http"
	"testing"
	"time"
)

type dummyReadDeadlineSetter struct {
	http.ResponseWriter
	called bool
}

func (rw *dummyReadDeadlineSetter) SetReadDeadline(deadline time.Time) error {
	rw.called = true
	return nil
}

func TestResponseController(t *testing.T) {
	rw := &dummyReadDeadlineSetter{}
	wrapped := wrap(&serverResponseTracer{rw: rw})
	rc := http.NewResponseController(wrapped)
	if err := rc.SetReadDeadline(time.Now()); err != nil {
		t.Errorf("SetReadDeadline() = %v; want nil", err)
	}
	if !rw.called {
		t.Error("SetReadDeadline() is not called")
	}
}
