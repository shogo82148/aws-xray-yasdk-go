package xrayhttp

import (
	"bufio"
	"net"
	"net/http"
	"testing"
)

type dummyCloseNotifier struct {
	http.ResponseWriter
}

func (dummyCloseNotifier) CloseNotify() <-chan bool {
	panic("unreachable")
}

func TestWrap_CloseNotify(t *testing.T) {
	got := wrap(&serverResponseTracer{rw: dummyCloseNotifier{}})
	if _, ok := got.(http.Flusher); ok {
		t.Error("want not to implement http.Flusher, but it does")
	}
	if _, ok := got.(http.CloseNotifier); !ok {
		t.Error("want to implement http.CloseNotifier, but it doesn't")
	}
	if _, ok := got.(http.Hijacker); ok {
		t.Error("want not to implement http.Hijacker, but it does")
	}
	if _, ok := got.(http.Pusher); ok {
		t.Error("want not to implement http.Pusher, but it does")
	}
}

type dummyHijacker struct {
	http.ResponseWriter
}

func (dummyHijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	panic("unreachable")
}

func TestWrap_Hijacker(t *testing.T) {
	got := wrap(&serverResponseTracer{rw: dummyHijacker{}})
	if _, ok := got.(http.Flusher); ok {
		t.Error("want not to implement http.Flusher, but it does")
	}
	if _, ok := got.(http.CloseNotifier); ok {
		t.Error("want not to implement http.CloseNotifier, but it does")
	}
	if _, ok := got.(http.Hijacker); !ok {
		t.Error("want to implement http.Hijacker, but it doesn't")
	}
	if _, ok := got.(http.Pusher); ok {
		t.Error("want not to implement http.Pusher, but it does")
	}
}

type dummyPusher struct {
	http.ResponseWriter
}

func (dummyPusher) Push(target string, opts *http.PushOptions) error {
	panic("unreachable")
}

func TestWrap_Pusher(t *testing.T) {
	got := wrap(&serverResponseTracer{rw: dummyPusher{}})
	if _, ok := got.(http.Flusher); ok {
		t.Error("want not to implement http.Flusher, but it does")
	}
	if _, ok := got.(http.CloseNotifier); ok {
		t.Error("want not to implement http.CloseNotifier, but it does")
	}
	if _, ok := got.(http.Hijacker); ok {
		t.Error("want not to implement http.Hijacker, but it does")
	}
	if _, ok := got.(http.Pusher); !ok {
		t.Error("want to implement http.Pusher, but it doesn't")
	}
}
