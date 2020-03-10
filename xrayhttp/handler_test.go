package xrayhttp

import "net/http"

// compile time checking to satisfy the interface
// https://golang.org/doc/effective_go.html#blank_implements
var _ http.ResponseWriter = (*responseTracer)(nil)
