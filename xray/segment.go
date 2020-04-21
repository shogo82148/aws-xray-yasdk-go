package xray

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/shogo82148/aws-xray-yasdk-go/xray/sampling"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/xraylog"
)

var nowFunc func() time.Time = time.Now

// contextKey is a value for use with context.WithValue. It's used as
// a pointer so it fits in an interface{} without allocation.
type contextKey struct {
	name string
}

func (k *contextKey) String() string { return "xray context value " + k.name }

var (
	segmentContextKey = &contextKey{"segment"}
	clientContextKey  = &contextKey{"client"}
)

type segmentStatus int

const (
	segmentStatusInit segmentStatus = iota
	segmentStatusEmitted
)

// Given several enabled plugins, the recorder should resolve a single one that's most representative of this environment
// Resolution order: EB > EKS > ECS > EC2
// EKS > ECS because the ECS plugin checks for an environment variable whereas the EKS plugin checks for a kubernetes authentication file, which is a stronger enable condition
var originPriority = map[string]int{
	schema.OriginElasticBeanstalk: 0,
	schema.OriginEKSContainer:     1,
	schema.OriginECSContainer:     2,
	schema.OriginEC2Instance:      3,
}

func origin() string {
	var org string
	priority := -1
	for _, p := range getPlugins() {
		if o := p.Origin(); o != "" {
			p, ok := originPriority[o]
			if !ok || p > priority {
				org = o
			}
		}
	}
	return org
}

// segment name should match /\A[\p{L}\p{N}\p{Z}_.:\/%&#=+\-@]{1,200}\z/
func sanitizeSegmentName(name string) string {
	var builder strings.Builder
	builder.Grow(len(name))
	var length int
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsSpace(r) {
			builder.WriteRune(r)
		} else if r <= unicode.MaxASCII && strings.IndexByte(`_.:/%&#=+\-@`, byte(r)) >= 0 {
			builder.WriteRune(r)
		} else {
			// ignore invalid charactors
			continue
		}
		length++
		if length >= 200 {
			break
		}
	}
	return builder.String()
}

// Segment is a segment.
type Segment struct {
	mu        sync.RWMutex
	ctx       context.Context
	name      string
	id        string
	traceID   string
	startTime time.Time
	endTime   time.Time
	status    segmentStatus

	// result of sampling
	sampled  bool
	ruleName string

	// parent segment
	// if the segment is the root, the parent is nil.
	parent *Segment

	// set by NewSegmentFromHeader
	traceHeader TraceHeader

	// root segment
	// if the segment is the root, the root points the segment it self.
	root *Segment

	// subsegments that are not completed.
	subsegments []*Segment

	// statics of the subsegments, used in the root.
	totalSegments   int
	closedSegments  int
	emittedSegments int

	// error information
	error    bool
	throttle bool
	fault    bool
	cause    *schema.Cause

	namespace   string
	user        string
	origin      string
	metadata    map[string]interface{}
	annotations map[string]interface{}
	sql         *schema.SQL
	http        *schema.HTTP
	aws         schema.AWS
}

// NewTraceID generates a string format of random trace ID.
func NewTraceID() string {
	var r [12]byte
	_, err := rand.Read(r[:])
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("1-%08x-%x", nowFunc().Unix(), r)
}

// NewSegmentID generates a string format of segment ID.
func NewSegmentID() string {
	var r [8]byte
	_, err := rand.Read(r[:])
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", r)
}

// ContextSegment return the segment of current context.
func ContextSegment(ctx context.Context) *Segment {
	seg := ctx.Value(segmentContextKey)
	if seg == nil {
		return nil
	}
	return seg.(*Segment)
}

// WithSegment returns a new context with the existing segment.
func WithSegment(ctx context.Context, seg *Segment) context.Context {
	return context.WithValue(ctx, segmentContextKey, seg)
}

// BeginDummySegment creates a new segment that traces nothing.
func BeginDummySegment(ctx context.Context) (context.Context, *Segment) {
	return WithSegment(ctx, nil), nil
}

// BeginSegment creates a new Segment for a given name and context.
//
// Caller should close the segment when the work is done.
func BeginSegment(ctx context.Context, name string) (context.Context, *Segment) {
	return beginSegment(ctx, name, TraceHeader{}, nil)
}

// BeginSegmentWithRequest creates a new Segment for a given name and context.
// The trace id is set by the x-amzn-trace-id header of the request.
//
// Caller should close the segment when the work is done.
func BeginSegmentWithRequest(ctx context.Context, name string, r *http.Request) (context.Context, *Segment) {
	return beginSegment(ctx, name, TraceHeader{}, r)
}

// BeginSegmentWithHeader creates a new Segment for a given name, context, and trace header.
// It is used for recovering the trace context. e.g. the receiver component of Amazon SQS.
// https://docs.aws.amazon.com/xray/latest/devguide/xray-services-sqs.html#xray-services-sqs-retrieving
//
// Caller should close the segment when the work is done.
func BeginSegmentWithHeader(ctx context.Context, name, header string) (context.Context, *Segment) {
	return beginSegment(ctx, name, ParseTraceHeader(header), nil)
}

func beginSegment(ctx context.Context, name string, h TraceHeader, r *http.Request) (context.Context, *Segment) {
	seg := &Segment{
		ctx:           ctx,
		name:          sanitizeSegmentName(name),
		id:            NewSegmentID(),
		startTime:     nowFunc(),
		totalSegments: 1,
		origin:        origin(),
	}
	seg.root = seg

	if r != nil {
		// Sampling strategy for http calls
		h = ParseTraceHeader(r.Header.Get(TraceIDHeaderKey))
		switch h.SamplingDecision {
		case SamplingDecisionSampled:
			xraylog.Debug(ctx, "Incoming header decided: Sampled=true")
			seg.sampled = true
		case SamplingDecisionNotSampled:
			xraylog.Debug(ctx, "Incoming header decided: Sampled=false")
		default:
			client := seg.client()
			sd := client.samplingStrategy.ShouldTrace(&sampling.Request{
				Host:        r.Host,
				URL:         r.URL.Path,
				Method:      r.Method,
				ServiceName: seg.name,
				ServiceType: seg.origin,
			})
			seg.sampled = sd.Sample
			if sd.Rule != nil {
				seg.ruleName = *sd.Rule
			}
			xraylog.Debugf(ctx, "SamplingStrategy decided: %t", seg.sampled)
		}
	} else {
		client := seg.client()
		sd := client.samplingStrategy.ShouldTrace(nil)
		seg.sampled = sd.Sample
		if sd.Rule != nil {
			seg.ruleName = *sd.Rule
		}
		xraylog.Debugf(ctx, "SamplingStrategy decided: %t", seg.sampled)
	}
	if h.TraceID == "" {
		h.TraceID = NewTraceID()
	}
	if seg.sampled {
		h.SamplingDecision = SamplingDecisionSampled
	} else {
		return BeginDummySegment(ctx)
	}

	seg.traceID = h.TraceID
	seg.traceHeader = h

	return WithSegment(ctx, seg), seg
}

// BeginSubsegment creates a new Segment for a given name and context.
//
// Caller should close the segment when the work is done.
func BeginSubsegment(ctx context.Context, name string) (context.Context, *Segment) {
	now := nowFunc()

	value := ctx.Value(segmentContextKey)
	if value == nil {
		if header := ctx.Value(lambdaContextKey); header != nil {
			// trace header comes from the AWS Lambda context.
			return beginSubsegmentForLambda(ctx, header.(string), name)
		}

		client := defaultClient
		if c := ctx.Value(clientContextKey); c != nil {
			client = c.(*Client)
		}
		client.contextMissingStrategy.ContextMissing(ctx, "context missing for "+name)
		return ctx, nil
	}
	parent := value.(*Segment)
	if parent == nil {
		return ctx, nil
	}

	root := parent.root
	seg := &Segment{
		ctx:       ctx,
		name:      sanitizeSegmentName(name),
		id:        NewSegmentID(),
		parent:    parent,
		root:      root,
		traceID:   parent.traceID,
		startTime: now,
	}
	ctx = context.WithValue(ctx, segmentContextKey, seg)

	root.mu.Lock()
	defer root.mu.Unlock()
	if parent != root {
		parent.mu.Lock()
		defer parent.mu.Unlock()
	}
	root.totalSegments++
	parent.subsegments = append(parent.subsegments, seg)

	return ctx, seg
}

// Sampled returns whether the current segment is sampled.
func (seg *Segment) Sampled() bool {
	if seg == nil {
		return false
	}
	root := seg.root
	root.mu.Lock()
	defer root.mu.Unlock()
	return root.sampled
}

type errorPanic struct {
	err interface{}
}

func (err *errorPanic) Error() string {
	return fmt.Sprintf("%T: %v", err.err, err.err)
}

// Close closes the segment.
func (seg *Segment) Close() {
	if seg == nil {
		return
	}
	if seg.parent != nil {
		xraylog.Debugf(seg.ctx, "Closing subsegment named %s", seg.name)
	} else {
		xraylog.Debugf(seg.ctx, "Closing segment named %s", seg.name)
	}
	err := recover()
	seg.AddPanic(err)
	seg.close()
	if seg.Sampled() {
		seg.emit()
	}
	if err != nil {
		panic(err)
	}
}

func (seg *Segment) close() {
	root := seg.root
	root.mu.Lock()
	defer root.mu.Unlock()
	if seg != root {
		seg.mu.Lock()
		defer seg.mu.Unlock()
	}
	root.closedSegments++
	seg.endTime = nowFunc()
}

func (seg *Segment) isRoot() bool {
	return seg.parent == nil
}

func (seg *Segment) inProgress() bool {
	return seg.endTime.IsZero()
}

func (seg *Segment) emit() {
	seg.client().Emit(seg.ctx, seg)
}

func (seg *Segment) client() *Client {
	if seg == nil {
		return defaultClient
	}
	seg.mu.RLock()
	defer seg.mu.RUnlock()
	return ContextClient(seg.ctx)
}

func newExceptionID() string {
	var r [8]byte
	_, err := rand.Read(r[:])
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", r)
}

// AddError sets error.
func (seg *Segment) AddError(err error) bool {
	if seg == nil {
		return err != nil
	}
	if err == nil {
		return false
	}
	seg.mu.Lock()
	defer seg.mu.Unlock()

	seg.fault = true
	if seg.cause == nil {
		seg.cause = &schema.Cause{}
	}
	seg.cause.WorkingDirectory, _ = os.Getwd()
	seg.cause.Exceptions = append(seg.cause.Exceptions, schema.Exception{
		ID:      newExceptionID(),
		Type:    fmt.Sprintf("%T", err),
		Message: err.Error(),
	})
	return true
}

// AddError sets the segment of the current context an error.
func AddError(ctx context.Context, err error) bool {
	return ContextSegment(ctx).AddError(err)
}

// AddPanic adds the information about panic.
func (seg *Segment) AddPanic(err interface{}) bool {
	if seg == nil {
		return err != nil
	}
	if err == nil {
		return false
	}
	seg.AddError(&errorPanic{err: err})
	return true
}

// AddPanic is the shorthand of ContextSegment(ctx).AddPanic(err).
func AddPanic(ctx context.Context, err interface{}) bool {
	return ContextSegment(ctx).AddPanic(err)
}

// SetError sets error flag.
func (seg *Segment) SetError() {
	if seg == nil {
		return
	}
	seg.mu.Lock()
	defer seg.mu.Unlock()
	seg.error = true
}

// SetError sets error flag.
func SetError(ctx context.Context) {
	ContextSegment(ctx).SetError()
}

// SetThrottle sets throttle flag.
func (seg *Segment) SetThrottle() {
	if seg == nil {
		return
	}
	seg.mu.Lock()
	defer seg.mu.Unlock()
	seg.throttle = true
}

// SetThrottle sets error flag.
func SetThrottle(ctx context.Context) {
	ContextSegment(ctx).SetThrottle()
}

// SetFault sets fault flag.
func (seg *Segment) SetFault() {
	if seg == nil {
		return
	}
	seg.mu.Lock()
	defer seg.mu.Unlock()
	seg.fault = true
}

// SetFault sets fault flag.
func SetFault(ctx context.Context) {
	ContextSegment(ctx).SetFault()
}

// DownstreamHeader returns a header for passing to downstream calls.
func (seg *Segment) DownstreamHeader() TraceHeader {
	if seg == nil {
		return TraceHeader{}
	}
	seg.mu.RLock()
	defer seg.mu.RUnlock()
	h := seg.traceHeader
	h.TraceID = seg.traceID
	h.ParentID = seg.id
	return h
}

// DownstreamHeader returns a header for passing to downstream calls.
func DownstreamHeader(ctx context.Context) TraceHeader {
	return ContextSegment(ctx).DownstreamHeader()
}

// SetNamespace sets namespace
func (seg *Segment) SetNamespace(namespace string) {
	if seg == nil {
		return
	}
	seg.mu.Lock()
	defer seg.mu.Unlock()
	seg.namespace = namespace
}

// SetNamespace sets namespace
func SetNamespace(ctx context.Context, namespace string) {
	ContextSegment(ctx).SetNamespace(namespace)
}

// Namespace returns the namespace.
func (seg *Segment) Namespace() string {
	if seg == nil {
		return ""
	}
	seg.mu.RLock()
	defer seg.mu.RUnlock()
	return seg.namespace
}

// AddMetadata adds metadata.
func (seg *Segment) AddMetadata(key string, value interface{}) {
	seg.AddMetadataToNamespace("default", key, value)
}

// AddMetadata adds metadata.
func AddMetadata(ctx context.Context, key string, value interface{}) {
	ContextSegment(ctx).AddMetadataToNamespace("default", key, value)
}

// AddMetadataToNamespace adds metadata.
func (seg *Segment) AddMetadataToNamespace(namespace, key string, value interface{}) {
	if seg == nil {
		return
	}
	seg.mu.Lock()
	defer seg.mu.Unlock()
	if seg.metadata == nil {
		seg.metadata = map[string]interface{}{}
	}
	if seg.metadata[namespace] == nil {
		seg.metadata[namespace] = map[string]interface{}{}
	}
	if ns, ok := seg.metadata[namespace].(map[string]interface{}); ok {
		ns[key] = value
	}
}

// AddMetadataToNamespace adds metadata.
func AddMetadataToNamespace(ctx context.Context, namespace, key string, value interface{}) {
	ContextSegment(ctx).AddMetadata(key, value)
}

// SetSQL sets the information of SQL queries.
func (seg *Segment) SetSQL(sql *schema.SQL) {
	if seg == nil {
		return
	}
	seg.mu.Lock()
	defer seg.mu.Unlock()
	seg.sql = sql
}

// SetSQL sets the information of SQL queries.
func SetSQL(ctx context.Context, sql *schema.SQL) {
	ContextSegment(ctx).SetSQL(sql)
}

// SetHTTPRequest sets the information of HTTP requests.
func (seg *Segment) SetHTTPRequest(request *schema.HTTPRequest) {
	if seg == nil {
		return
	}
	seg.mu.Lock()
	defer seg.mu.Unlock()
	if seg.http == nil {
		seg.http = &schema.HTTP{}
	}
	seg.http.Request = request
}

// SetHTTPRequest sets the information of HTTP requests.
func SetHTTPRequest(ctx context.Context, request *schema.HTTPRequest) {
	ContextSegment(ctx).SetHTTPRequest(request)
}

// SetHTTPResponse sets the information of HTTP requests.
func (seg *Segment) SetHTTPResponse(response *schema.HTTPResponse) {
	if seg == nil {
		return
	}
	seg.mu.Lock()
	defer seg.mu.Unlock()
	if seg.http == nil {
		seg.http = &schema.HTTP{}
	}
	seg.http.Response = response
}

// SetHTTPResponse sets the information of HTTP requests.
func SetHTTPResponse(ctx context.Context, response *schema.HTTPResponse) {
	ContextSegment(ctx).SetHTTPResponse(response)
}

// SetAWS sets the information about the AWS resource on which your application served the request.
func (seg *Segment) SetAWS(awsData schema.AWS) {
	if seg == nil {
		return
	}
	seg.mu.Lock()
	defer seg.mu.Unlock()
	if seg.aws == nil {
		seg.aws = schema.AWS{}
	}
	for key, value := range awsData {
		seg.aws.Set(key, value)
	}
}

// SetAWS sets the information about the AWS resource on which your application served the request.
func SetAWS(ctx context.Context, awsData schema.AWS) {
	ContextSegment(ctx).SetAWS(awsData)
}

// SetUser sets a user id.
func (seg *Segment) SetUser(user string) {
	if seg == nil {
		return
	}
	seg.mu.Lock()
	defer seg.mu.Unlock()
	seg.user = user
}

// SetUser sets a user id.
func SetUser(ctx context.Context, user string) {
	ContextSegment(ctx).SetUser(user)
}

func (seg *Segment) addAnnotation(key string, value interface{}) {
	if seg == nil {
		return
	}
	seg.mu.Lock()
	defer seg.mu.Unlock()
	if seg.annotations == nil {
		seg.annotations = make(map[string]interface{})
	}
	seg.annotations[key] = value
}

// AddAnnotationBool adds a boolean type annotation.
func (seg *Segment) AddAnnotationBool(key string, value bool) {
	seg.addAnnotation(key, value)
}

// AddAnnotationBool adds a boolean type annotation.
func AddAnnotationBool(ctx context.Context, key string, value bool) {
	ContextSegment(ctx).addAnnotation(key, value)
}

// AddAnnotationString adds a string type annotation.
func (seg *Segment) AddAnnotationString(key, value string) {
	seg.addAnnotation(key, value)
}

// AddAnnotationString adds a string type annotation.
func AddAnnotationString(ctx context.Context, key, value string) {
	ContextSegment(ctx).addAnnotation(key, value)
}

// AddAnnotationInt64 adds a 64 bit integer type annotation.
func (seg *Segment) AddAnnotationInt64(key string, value int64) {
	seg.addAnnotation(key, value)
}

// AddAnnotationInt64 adds a 64 bit integer type annotation.
func AddAnnotationInt64(ctx context.Context, key string, value int64) {
	ContextSegment(ctx).addAnnotation(key, value)
}

// AddAnnotationUint64 adds a 64 bit integer type annotation.
func (seg *Segment) AddAnnotationUint64(key string, value uint64) {
	seg.addAnnotation(key, value)
}

// AddAnnotationUint64 adds a 64 bit integer type annotation.
func AddAnnotationUint64(ctx context.Context, key string, value uint64) {
	ContextSegment(ctx).addAnnotation(key, value)
}

// AddAnnotationFloat64 adds a 64 bit integer type annotation.
func (seg *Segment) AddAnnotationFloat64(key string, value float64) {
	seg.addAnnotation(key, value)
}

// AddAnnotationFloat64 adds a 64 bit integer type annotation.
func AddAnnotationFloat64(ctx context.Context, key string, value float64) {
	ContextSegment(ctx).addAnnotation(key, value)
}
