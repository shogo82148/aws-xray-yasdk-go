package xrayaws

import (
	"context"
	"sync"

	_ "github.com/aws/aws-sdk-go-v2/aws"
	_ "github.com/shogo82148/aws-xray-yasdk-go/xray"
	_ "github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
	"github.com/shogo82148/aws-xray-yasdk-go/xrayaws-v2/whitelist"
	_ "github.com/shogo82148/aws-xray-yasdk-go/xrayhttp"
)

//go:generate go run codegen.go

type subsegments struct {
	mu  sync.Mutex
	ctx context.Context
}

func contextSubsegments(ctx context.Context) *subsegments {
	segs := ctx.Value(segmentsContextKey)
	if segs == nil {
		return nil
	}
	return segs.(*subsegments)
}

// contextKey is a value for use with context.WithValue. It's used as
// a pointer so it fits in an interface{} without allocation.
type contextKey struct {
	name string
}

func (k *contextKey) String() string { return "xrayaws-v2 context value " + k.name }

var segmentsContextKey = &contextKey{"segments"}

// Client adds X-Ray tracing to an AWS client.
func Client(c interface{}) interface{} {
	if c == nil {
		panic("Please initialize the provided AWS client before passing to the Client() method.")
	}
	panic("TODO: implement me")
}

// ClientWithWhitelist adds X-Ray tracing to an AWS client with custom whitelist.
func ClientWithWhitelist(c interface{}, whitelist *whitelist.Whitelist) interface{} {
	if c == nil {
		panic("Please initialize the provided AWS client before passing to the Client() method.")
	}
	panic("TODO: implement me")
}
