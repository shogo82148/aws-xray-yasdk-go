package xrayaws

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"time"

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
	mu sync.Mutex

	name           string
	initializeTime time.Time
	marshalTime    time.Time

	ctx           context.Context
	awsCtx        context.Context
	awsSeg        *xray.Segment
	attemptCtx    context.Context
	attemptSeg    *xray.Segment
	attemptCancel context.CancelFunc
	unmarshalCtx  context.Context
	unmarshalSeg  *xray.Segment
}

func (segs *subsegments) closeRoot() {
	segs.mu.Lock()
	defer segs.mu.Unlock()

	if segs.awsCtx != nil {
		segs.awsSeg.Close()
		segs.awsCtx, segs.awsSeg = nil, nil
	}
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
	if segs.unmarshalCtx != nil {
		segs.unmarshalSeg.Close()
		segs.unmarshalCtx, segs.unmarshalSeg = nil, nil
	}
}

type xrayMiddleware struct {
	o *option
}

func (xrayMiddleware) ID() string {
	return "XRayMiddleware"
}

func (m xrayMiddleware) HandleInitialize(
	ctx context.Context, in middleware.InitializeInput, next middleware.InitializeHandler,
) (
	out middleware.InitializeOutput, metadata middleware.Metadata, err error,
) {
	segs := &subsegments{
		name: m.o.name,
		ctx:  ctx,
	}
	ctx = context.WithValue(ctx, segmentsContextKey, segs)
	segs.initializeTime = time.Now()

	defer segs.closeRoot()       // make sure root segment closed
	defer segs.closeExceptRoot() // make share all segments closed

	out, metadata, err = next.HandleInitialize(ctx, in)
	if err != nil {
		segs.awsSeg.AddError(&smithy.OperationError{
			ServiceID:     awsmiddle.GetServiceID(ctx),
			OperationName: awsmiddle.GetOperationName(ctx),
			Err:           err,
		})
	}
	if segs.awsSeg != nil {
		aws := schema.AWS{}
		m.o.insertParameter(aws, segs.name, awsmiddle.GetOperationName(ctx), in.Parameters, out.Result)
		segs.awsSeg.SetNamespace("aws")
		segs.awsSeg.SetAWS(aws)
	}
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
		segs.marshalTime = time.Now()
		segs.mu.Unlock()
	}
	return next.HandleSerialize(ctx, in)
}

type endMarshalMiddleware struct{}

func (endMarshalMiddleware) ID() string {
	return "XRayEndMarshalMiddleware"
}

func (m endMarshalMiddleware) HandleFinalize(ctx context.Context, in middleware.FinalizeInput, next middleware.FinalizeHandler) (
	out middleware.FinalizeOutput, metadata middleware.Metadata, err error,
) {
	if segs := contextSubsegments(ctx); segs != nil {
		segs.mu.Lock()

		if segs.name == "" {
			segs.name = getServiceName(ctx, in)
		}
		segs.awsCtx, segs.awsSeg = xray.BeginSubsegmentAt(ctx, segs.initializeTime, segs.name)

		_, marshalSeg := xray.BeginSubsegmentAt(segs.awsCtx, segs.marshalTime, "marshal")
		marshalSeg.Close()
		segs.mu.Unlock()
	}
	return next.HandleFinalize(ctx, in)
}

func getServiceName(ctx context.Context, in middleware.FinalizeInput) string {
	if name := awsmiddle.GetSigningName(ctx); name != "" {
		return name
	}
	req, ok := in.Request.(*smithyhttp.Request)
	if !ok {
		return awsmiddle.GetServiceID(ctx)
	}

	const prefix = "AWS4-HMAC-SHA256 "
	auth := req.Header.Get("Authorization")
	if len(auth) < len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) {
		return awsmiddle.GetServiceID(ctx)
	}
	auth = auth[len(prefix):]
	parts := strings.Split(auth, ",")
	for _, part := range parts {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "Credential" {
			continue
		}

		cred := strings.Split(value, "/")
		if len(cred) < 4 {
			return awsmiddle.GetServiceID(ctx)
		}
		return cred[3]
	}

	return awsmiddle.GetServiceID(ctx)
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
// a pointer so it fits in an any without allocation.
type contextKey struct {
	name string
}

func (k *contextKey) String() string { return "xrayaws-v2 context value " + k.name }

var segmentsContextKey = &contextKey{"segments"}

// WithXRay is the X-Ray tracing option.
func WithXRay() config.LoadOptionsFunc {
	return WithServiceName("", defaultWhitelist)
}

// WithWhitelist returns a X-Ray tracing option with custom whitelist.
func WithWhitelist(whitelist *whitelist.Whitelist) config.LoadOptionsFunc {
	return WithServiceName("", whitelist)
}

// WithServiceName returns a X-Ray tracing option with custom service name.
func WithServiceName(name string, whitelist *whitelist.Whitelist) config.LoadOptionsFunc {
	return func(o *config.LoadOptions) error {
		newOption := option{
			name:      name,
			whitelist: whitelist,
		}
		o.APIOptions = append(
			o.APIOptions,
			newOption.addMiddleware,
		)
		return nil
	}
}

type option struct {
	name      string
	whitelist *whitelist.Whitelist
}

func (o *option) addMiddleware(stack *middleware.Stack) error {
	stack.Initialize.Add(xrayMiddleware{o: o}, middleware.After)
	stack.Serialize.Add(beginMarshalMiddleware{}, middleware.Before)
	stack.Finalize.Insert(endMarshalMiddleware{}, "Signing", middleware.After)
	stack.Deserialize.Add(beginAttemptMiddleware{}, middleware.Before)
	stack.Deserialize.Insert(endAttemptMiddleware{}, "OperationDeserializer", middleware.After)
	return nil
}

func (o *option) insertParameter(aws schema.AWS, serviceName, operationName string, params, result any) {
	if o.whitelist == nil {
		return
	}
	service, ok := o.whitelist.Services[serviceName]
	if !ok {
		return
	}
	operation, ok := service.Operations[operationName]
	if !ok {
		return
	}
	for _, key := range operation.RequestParameters {
		aws.Set(key, getValue(params, key))
	}
	for key, desc := range operation.RequestDescriptors {
		insertDescriptor(desc, aws, params, key)
	}
	for _, key := range operation.ResponseParameters {
		aws.Set(key, getValue(result, key))
	}
	for key, desc := range operation.ResponseDescriptors {
		insertDescriptor(desc, aws, result, key)
	}
}

func getValue(v any, key string) any {
	v1 := reflect.ValueOf(v)
	if v1.Kind() == reflect.Ptr {
		v1 = v1.Elem()
	}
	if v1.Kind() != reflect.Struct {
		return nil
	}
	typ := v1.Type()

	for i := 0; i < v1.NumField(); i++ {
		if typ.Field(i).Name == key {
			return v1.Field(i).Interface()
		}
	}
	return nil
}

func insertDescriptor(desc *whitelist.Descriptor, aws schema.AWS, v any, key string) {
	renameTo := desc.RenameTo
	if renameTo == "" {
		renameTo = key
	}
	value := getValue(v, key)
	switch {
	case desc.Map:
		if !desc.GetKeys {
			return
		}
		val := reflect.ValueOf(value)
		if val.Kind() != reflect.Map {
			return
		}
		keySlice := make([]any, 0, val.Len())
		for _, key := range val.MapKeys() {
			keySlice = append(keySlice, key.Interface())
		}
		aws.Set(renameTo, keySlice)
	case desc.List:
		if !desc.GetCount {
			return
		}
		val := reflect.ValueOf(value)
		if kind := val.Kind(); kind != reflect.Slice && kind != reflect.Array {
			return
		}
		aws.Set(renameTo, val.Len())
	default:
		aws.Set(renameTo, value)
	}
}
