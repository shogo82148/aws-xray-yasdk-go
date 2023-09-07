//go:build !go1.18
// +build !go1.18

package xrayaws

// any type is available from Go 1.18.
// It is for compatibility for Go 1.17 or previous Versions.
type any = interface{}
