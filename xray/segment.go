package xray

import (
	"crypto/rand"
	"fmt"
	"time"
)

// Segment is a segment.
type Segment struct {
}

// NewTraceID generates a string format of random trace ID.
func NewTraceID() string {
	var r [12]byte
	_, err := rand.Read(r[:])
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("1-%08x-%02x", time.Now().Unix(), r)
}
