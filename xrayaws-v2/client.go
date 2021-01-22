package xrayaws

import (
	"context"
	"log"
	"sync"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/smithy-go/middleware"
	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	_ "github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
	"github.com/shogo82148/aws-xray-yasdk-go/xrayaws-v2/whitelist"
	_ "github.com/shogo82148/aws-xray-yasdk-go/xrayhttp"
)

//go:generate go run codegen.go

type subsegments struct {
	mu            sync.Mutex
	ctx           context.Context
	awsCtx        context.Context
	awsSeg        *xray.Segment
	marshalCtx    context.Context
	marshalSeg    *xray.Segment
	attemptCtx    context.Context
	attemptSeg    *xray.Segment
	attemptCancel context.CancelFunc
	unmarshalCtx  context.Context
	unmarshalSeg  *xray.Segment
}

// close all segments except root.
func (segs *subsegments) closeExceptRoot() {
	segs.mu.Lock()
	defer segs.mu.Unlock()

	if segs.attemptCtx != nil {
		segs.attemptCancel()
		segs.attemptSeg.Close()
		segs.attemptCancel = nil
		segs.attemptCtx, segs.attemptSeg = nil, nil
	}
	if segs.marshalCtx != nil {
		segs.marshalSeg.Close()
		segs.marshalCtx, segs.marshalSeg = nil, nil
	}
	if segs.unmarshalCtx != nil {
		segs.unmarshalSeg.Close()
		segs.unmarshalCtx, segs.unmarshalSeg = nil, nil
	}
}

type xrayMiddleware struct{}

func (xrayMiddleware) ID() string {
	return "XRayMiddleware"
}

func (xrayMiddleware) HandleInitialize(
	ctx context.Context, in middleware.InitializeInput, next middleware.InitializeHandler,
) (
	out middleware.InitializeOutput, metadata middleware.Metadata, err error,
) {
	segs := &subsegments{
		ctx: ctx,
	}
	ctx = context.WithValue(ctx, segmentsContextKey, segs)
	segs.awsCtx, segs.awsSeg = xray.BeginSubsegment(ctx, "TODO")
	defer segs.awsSeg.Close()
	defer segs.closeExceptRoot() // make share all segments closed
	segs.awsSeg.SetNamespace("aws")

	out, metadata, err = next.HandleInitialize(ctx, in)
	segs.closeExceptRoot()

	// TODO: record result

	return
}

type beginMarshalMiddleware struct{}

func (beginMarshalMiddleware) ID() string {
	return "XRayBeginMarshalMiddleware"
}

func (beginMarshalMiddleware) HandleSerialize(
	ctx context.Context, in middleware.SerializeInput, next middleware.SerializeHandler,
) (
	out middleware.SerializeOutput, metadata middleware.Metadata, err error,
) {
	if segs := contextSubsegments(ctx); segs != nil {
		segs.mu.Lock()
		segs.marshalCtx, segs.marshalSeg = xray.BeginSubsegment(segs.awsCtx, "marshal")
		segs.mu.Unlock()
	}
	return next.HandleSerialize(ctx, in)
}

type endMarshalMiddleware struct{}

func (endMarshalMiddleware) ID() string {
	return "XRayEndMarshalMiddleware"
}

func (endMarshalMiddleware) HandleBuild(
	ctx context.Context, in middleware.BuildInput, next middleware.BuildHandler,
) (
	out middleware.BuildOutput, metadata middleware.Metadata, err error,
) {
	if segs := contextSubsegments(ctx); segs != nil {
		segs.mu.Lock()
		if segs.marshalCtx != nil {
			segs.marshalSeg.Close()
			segs.marshalCtx, segs.marshalSeg = nil, nil
		}
		segs.mu.Unlock()
	}
	return next.HandleBuild(ctx, in)
}

type finalizeMiddleware struct{}

func (finalizeMiddleware) ID() string {
	return "XRayEndMarshalMiddleware"
}

func (finalizeMiddleware) HandleFinalize(
	ctx context.Context, in middleware.FinalizeInput, next middleware.FinalizeHandler,
) (
	out middleware.FinalizeOutput, metadata middleware.Metadata, err error,
) {
	log.Printf("%#v", in.Request)
	defer log.Println("end finalize")
	return next.HandleFinalize(ctx, in)
}

type beginUnmarshalMiddleware struct{}

func (beginUnmarshalMiddleware) ID() string {
	return "XRayBeginUnmarshalMiddleware"
}

func (beginUnmarshalMiddleware) HandleDeserialize(
	ctx context.Context, in middleware.DeserializeInput, next middleware.DeserializeHandler,
) (
	out middleware.DeserializeOutput, metadata middleware.Metadata, err error,
) {
	if segs := contextSubsegments(ctx); segs != nil {
		segs.mu.Lock()
		segs.unmarshalCtx, segs.unmarshalSeg = xray.BeginSubsegment(segs.awsCtx, "unmarshal")
		segs.mu.Unlock()
	}
	return next.HandleDeserialize(ctx, in)
}

type endUnmarshalMiddleware struct{}

func (endUnmarshalMiddleware) ID() string {
	return "XRayBeginUnmarshalMiddleware"
}

func (endUnmarshalMiddleware) HandleDeserialize(
	ctx context.Context, in middleware.DeserializeInput, next middleware.DeserializeHandler,
) (
	out middleware.DeserializeOutput, metadata middleware.Metadata, err error,
) {
	if segs := contextSubsegments(ctx); segs != nil {
		segs.mu.Lock()
		if segs.unmarshalCtx != nil {
			segs.unmarshalSeg.Close()
			segs.unmarshalCtx, segs.unmarshalSeg = nil, nil
		}
		segs.mu.Unlock()
	}
	return next.HandleDeserialize(ctx, in)
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

// WithXRay is the X-Ray tracing option.
func WithXRay() config.LoadOptionsFunc {
	return WithWhitelist(defaultWhitelist)
}

// WithWhitelist returns a X-Ray tracing option with custom whitelist.
func WithWhitelist(whitelist *whitelist.Whitelist) config.LoadOptionsFunc {
	return func(o *config.LoadOptions) error {
		newOption := option{whitelist: whitelist}
		o.APIOptions = append(
			o.APIOptions,
			newOption.addMiddleware,
		)
		return nil
	}
}

type option struct {
	whitelist *whitelist.Whitelist
}

func (o option) addMiddleware(stack *middleware.Stack) error {
	stack.Initialize.Add(xrayMiddleware{}, middleware.Before)
	stack.Serialize.Add(beginMarshalMiddleware{}, middleware.Before)
	stack.Build.Add(endMarshalMiddleware{}, middleware.After)
	stack.Finalize.Add(finalizeMiddleware{}, middleware.Before)
	stack.Deserialize.Add(beginUnmarshalMiddleware{}, middleware.Before)
	stack.Deserialize.Add(endUnmarshalMiddleware{}, middleware.After)
	return nil
}
