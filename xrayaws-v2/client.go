package xrayaws

import (
	"context"
	"sync"

	awsmiddle "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/smithy-go"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
	"github.com/shogo82148/aws-xray-yasdk-go/xrayaws-v2/whitelist"
	"github.com/shogo82148/aws-xray-yasdk-go/xrayhttp"
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
	segs.awsCtx, segs.awsSeg = xray.BeginSubsegment(ctx, awsmiddle.GetSigningName(ctx))
	defer segs.awsSeg.Close()
	defer segs.closeExceptRoot() // make share all segments closed
	segs.awsSeg.SetNamespace("aws")

	out, metadata, err = next.HandleInitialize(ctx, in)
	if err != nil {
		segs.awsSeg.AddError(&smithy.OperationError{
			ServiceID:     awsmiddle.GetServiceID(ctx),
			OperationName: awsmiddle.GetOperationName(ctx),
			Err:           err,
		})
	}
	segs.closeExceptRoot()

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

type beginAttemptMiddleware struct{}

func (beginAttemptMiddleware) ID() string {
	return "XRayBeginAttemptMiddleware"
}

func (beginAttemptMiddleware) HandleDeserialize(
	ctx context.Context, in middleware.DeserializeInput, next middleware.DeserializeHandler,
) (
	out middleware.DeserializeOutput, metadata middleware.Metadata, err error,
) {
	segs := contextSubsegments(ctx)
	if segs != nil {
		segs.mu.Lock()
		segs.attemptCtx, segs.attemptSeg = xray.BeginSubsegment(segs.awsCtx, "attempt")
		var cancel context.CancelFunc
		ctx, cancel = xrayhttp.WithClientTrace(segs.attemptCtx)
		segs.attemptCancel = cancel
		segs.mu.Unlock()
	}
	out, metadata, err = next.HandleDeserialize(ctx, in)
	if segs != nil {
		segs.mu.Lock()
		if segs.unmarshalCtx != nil {
			segs.unmarshalSeg.AddError(err)
			segs.unmarshalSeg.Close()
			segs.unmarshalCtx, segs.unmarshalSeg = nil, nil
		}
		segs.mu.Unlock()
		if smithyResponse, ok := out.RawResponse.(*smithyhttp.Response); ok {
			resp := smithyResponse.Response
			segs.awsSeg.SetHTTPResponse(&schema.HTTPResponse{
				Status:        resp.StatusCode,
				ContentLength: resp.ContentLength,
			})
		}
	}
	return
}

type endAttemptMiddleware struct{}

func (endAttemptMiddleware) ID() string {
	return "XRayEndAttemptMiddleware"
}

func (endAttemptMiddleware) HandleDeserialize(
	ctx context.Context, in middleware.DeserializeInput, next middleware.DeserializeHandler,
) (
	out middleware.DeserializeOutput, metadata middleware.Metadata, err error,
) {
	segs := contextSubsegments(ctx)
	if segs != nil {
		aws := schema.AWS{
			"operation": awsmiddle.GetOperationName(ctx),
			"region":    awsmiddle.GetRegion(ctx),
		}
		if smithyRequest, ok := in.Request.(*smithyhttp.Request); ok {
			req := smithyRequest.Request
			aws["request_id"] = req.Header.Get("Amz-Sdk-Invocation-Id")
		}
		segs.awsSeg.SetAWS(aws)
	}
	out, metadata, err = next.HandleDeserialize(ctx, in)
	if segs != nil {
		segs.mu.Lock()
		if segs.attemptCtx != nil {
			if err != nil {
				// r.Error will be stored into segs.awsSeg,
				// so we just set fault here.
				segs.attemptSeg.SetFault()
			}
			segs.attemptSeg.Close()
			segs.attemptCancel()
			segs.attemptCtx, segs.attemptSeg = nil, nil
			segs.attemptCancel = nil
		}
		if err == nil {
			segs.unmarshalCtx, segs.unmarshalSeg = xray.BeginSubsegment(segs.awsCtx, "unmarshal")
		}
		segs.mu.Unlock()
	}
	return
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
	stack.Initialize.Add(xrayMiddleware{}, middleware.After)
	stack.Serialize.Add(beginMarshalMiddleware{}, middleware.Before)
	stack.Build.Add(endMarshalMiddleware{}, middleware.After)
	stack.Deserialize.Add(beginAttemptMiddleware{}, middleware.Before)
	stack.Deserialize.Insert(endAttemptMiddleware{}, "OperationDeserializer", middleware.After)
	return nil
}
