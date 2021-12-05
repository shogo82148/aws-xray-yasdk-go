package xray

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

var xrayData = schema.AWS{
	"xray": map[string]interface{}{
		"sdk_version": Version,
		"sdk":         Name,
	},
}

// mock time function
func fixedTime() time.Time { return time.Date(2001, time.September, 9, 1, 46, 40, 0, time.UTC) }

func TestNewTraceID(t *testing.T) {
	id := NewTraceID()
	pattern := `^1-[0-9a-fA-F]{8}-[0-9a-fA-F]{24}$`
	if matched, err := regexp.MatchString(pattern, id); err != nil || !matched {
		t.Errorf("id should match %q, but got %q", pattern, id)
	}
}

func BenchmarkNewTraceID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		runtime.KeepAlive(NewTraceID())
	}
}

func TestNewSegmentID(t *testing.T) {
	id := NewSegmentID()
	pattern := `^[0-9a-fA-F]{16}$`
	if matched, err := regexp.MatchString(pattern, id); err != nil || !matched {
		t.Errorf("id should match %q, but got %q", pattern, id)
	}
}

func BenchmarkNewSegmentID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		runtime.KeepAlive(NewSegmentID())
	}
}

func TestSanitizeSegmentName(t *testing.T) {
	tc := []struct {
		in   string
		want string
	}{
		{in: "abc 123", want: "abc 123"},
		{in: `_.:/%&#=+\-@`, want: `_.:/%&#=+\-@`},
		{in: "abc!?[](){}123", want: "abc123"},
		{in: "こんにちは世界", want: "こんにちは世界"},
		{in: strings.Repeat("あ", 201), want: strings.Repeat("あ", 200)},
	}
	for _, tt := range tc {
		got := sanitizeSegmentName(tt.in)
		if got != tt.want {
			t.Errorf("%q: want %q, got %q", tt.in, tt.want, got)
		}
	}
}

func TestBeginSegment(t *testing.T) {
	nowFunc = fixedTime
	defer func() { nowFunc = time.Now }()

	ctx, td := NewTestDaemon(nil)
	defer td.Close()

	ctx, seg := BeginSegment(ctx, "foobar")
	_ = ctx // do something using ctx
	seg.Close()

	got, err := td.Recv()
	if err != nil {
		t.Error(err)
	}
	want := &schema.Segment{
		Name:      "foobar",
		ID:        seg.id,
		TraceID:   seg.traceID,
		StartTime: 1000000000,
		EndTime:   1000000000,
		Service:   ServiceData,
		AWS:       xrayData,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestBeginSegmentWithRequest(t *testing.T) {
	nowFunc = fixedTime
	defer func() { nowFunc = time.Now }()

	ctx, td := NewTestDaemon(nil)
	defer td.Close()

	req, err := http.NewRequest(http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(TraceIDHeaderKey, "Root=1-5e645f3e-1dfad076a177c5ccc5de12f5;Parent=03babb4ba280be51;foo=bar")
	ctx, seg := BeginSegmentWithRequest(ctx, "foobar", req)
	_ = ctx // do something using ctx
	seg.Close()

	got, err := td.Recv()
	if err != nil {
		t.Error(err)
	}
	want := &schema.Segment{
		Name:      "foobar",
		ID:        seg.id,
		TraceID:   "1-5e645f3e-1dfad076a177c5ccc5de12f5",
		StartTime: 1000000000,
		EndTime:   1000000000,
		ParentID:  "03babb4ba280be51",
		Type:      "subsegment",
		Service:   ServiceData,
		AWS:       xrayData,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestBeginSegmentWithRequest_NotSampled(t *testing.T) {
	nowFunc = fixedTime
	defer func() { nowFunc = time.Now }()

	ctx, td := NewTestDaemon(nil)
	defer td.Close()

	req, err := http.NewRequest(http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	// trace header with "Sampled=0"
	req.Header.Set(TraceIDHeaderKey, "Root=1-5e645f3e-1dfad076a177c5ccc5de12f5;Sampled=0")
	ctx, seg := BeginSegmentWithRequest(ctx, "foobar", req)
	_ = ctx // do something using ctx
	seg.Close()

	if _, err := td.Recv(); err == nil {
		t.Error("want timeout, but not")
	}
}

func TestBeginSegmentWithRequest_Sampled(t *testing.T) {
	nowFunc = fixedTime
	defer func() { nowFunc = time.Now }()

	ctx, td := NewTestDaemon(nil)
	defer td.Close()

	req, err := http.NewRequest(http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	// trace header with "Sampled=0"
	req.Header.Set(TraceIDHeaderKey, "Root=1-5e645f3e-1dfad076a177c5ccc5de12f5;Sampled=1")
	ctx, seg := BeginSegmentWithRequest(ctx, "foobar", req)
	_ = ctx // do something using ctx
	seg.Close()

	got, err := td.Recv()
	if err != nil {
		t.Error(err)
	}
	want := &schema.Segment{
		Name:      "foobar",
		ID:        seg.id,
		TraceID:   "1-5e645f3e-1dfad076a177c5ccc5de12f5",
		StartTime: 1000000000,
		EndTime:   1000000000,
		Service:   ServiceData,
		AWS:       xrayData,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestBeginSubsegment(t *testing.T) {
	nowFunc = fixedTime
	defer func() { nowFunc = time.Now }()

	ctx, td := NewTestDaemon(nil)
	defer td.Close()

	ctx, root := BeginSegment(ctx, "root")
	ctx, seg := BeginSubsegment(ctx, "subsegment")
	_ = ctx // do something using ctx
	seg.Close()
	root.Close()

	got, err := td.Recv()
	if err != nil {
		t.Error(err)
	}
	want := &schema.Segment{
		Name:      "root",
		ID:        root.id,
		TraceID:   root.traceID,
		StartTime: 1000000000,
		EndTime:   1000000000,
		Subsegments: []*schema.Segment{
			{
				Name:      "subsegment",
				ID:        seg.id,
				StartTime: 1000000000,
				EndTime:   1000000000,
			},
		},
		Service: ServiceData,
		AWS:     xrayData,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestBeginSubsegment_contextMissing(t *testing.T) {
	var ctxMissing interface{}

	ctx, td := NewTestDaemon(nil)
	defer td.Close()
	td.ContextMissing = func(ctx context.Context, v interface{}) {
		ctxMissing = v
	}

	ctx, seg := BeginSubsegment(ctx, "subsegment")
	_ = ctx // do something using ctx
	seg.Close()

	if ctxMissing == nil {
		t.Error("want not nil, got nil")
	}
}

func TestSegmentPanic(t *testing.T) {
	nowFunc = fixedTime
	defer func() { nowFunc = time.Now }()

	ctx, td := NewTestDaemon(nil)
	defer td.Close()

	errPanic := errors.New("PANIC")
	var id, traceID string
	func() {
		defer func() {
			err := recover()
			if err != errPanic {
				t.Errorf("want %v, got %v", errPanic, err)
			}
		}()
		func() {
			_, seg := BeginSegment(ctx, "foobar")
			defer seg.Close()
			id = seg.id
			traceID = seg.traceID
			panic(errPanic)
		}()
	}()

	got, err := td.Recv()
	if err != nil {
		t.Error(err)
	}
	want := &schema.Segment{
		Name:      "foobar",
		ID:        id,
		TraceID:   traceID,
		StartTime: 1000000000,
		EndTime:   1000000000,
		Fault:     true,
		Cause: &schema.Cause{
			WorkingDirectory: got.Cause.WorkingDirectory,
			Exceptions: []schema.Exception{
				{
					ID:      got.Cause.Exceptions[0].ID,
					Message: "*errors.errorString: PANIC",
					Type:    "*xray.errorPanic",
				},
			},
		},
		Service: ServiceData,
		AWS:     xrayData,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestAddError(t *testing.T) {
	nowFunc = fixedTime
	defer func() { nowFunc = time.Now }()

	ctx, td := NewTestDaemon(nil)
	defer td.Close()

	ctx, seg := BeginSegment(ctx, "foobar")
	if !AddError(ctx, errors.New("some error")) {
		t.Error("want true, got false")
	}
	seg.Close()

	got, err := td.Recv()
	if err != nil {
		t.Error(err)
	}
	want := &schema.Segment{
		Name:      "foobar",
		ID:        seg.id,
		TraceID:   seg.traceID,
		StartTime: 1000000000,
		EndTime:   1000000000,
		Fault:     true,
		Cause: &schema.Cause{
			WorkingDirectory: got.Cause.WorkingDirectory,
			Exceptions: []schema.Exception{
				{
					ID:      got.Cause.Exceptions[0].ID,
					Message: "some error",
					Type:    "*errors.errorString",
				},
			},
		},
		Service: ServiceData,
		AWS:     xrayData,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestDownstreamHeader(t *testing.T) {
	ctx, td := NewTestDaemon(nil)
	defer td.Close()

	ctx, seg := BeginSegment(ctx, "foobar")
	defer seg.Close()
	h := DownstreamHeader(ctx)
	if h.TraceID != seg.traceID {
		t.Errorf("invalid trace id: want %s, got %s", seg.traceID, h.TraceID)
	}
	if h.ParentID != seg.id {
		t.Errorf("invalid parent id: want %s, got %s", seg.id, h.ParentID)
	}
}

func TestDownstreamHeader_InheritUpstream(t *testing.T) {
	ctx, td := NewTestDaemon(nil)
	defer td.Close()

	req, err := http.NewRequest(http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(TraceIDHeaderKey, "Root=1-5e645f3e-1dfad076a177c5ccc5de12f5;Parent=03babb4ba280be51;foo=bar")
	ctx, seg := BeginSegmentWithRequest(ctx, "foobar", req)
	defer seg.Close()
	h := DownstreamHeader(ctx)
	if h.TraceID != "1-5e645f3e-1dfad076a177c5ccc5de12f5" {
		t.Errorf("invalid trace id: want %s, got %s", "1-5e645f3e-1dfad076a177c5ccc5de12f5", h.TraceID)
	}
	if h.ParentID != seg.id {
		t.Errorf("invalid parent id: want %s, got %s", seg.id, h.ParentID)
	}
	if h.AdditionalData["foo"] != "bar" {
		t.Errorf("invalid additional data: want %s, got %s", "bar", h.AdditionalData["foo"])
	}
}

func TestSegment_SetUser(t *testing.T) {
	nowFunc = fixedTime
	defer func() { nowFunc = time.Now }()

	ctx, td := NewTestDaemon(nil)
	defer td.Close()

	ctx, seg := BeginSegment(ctx, "foobar")
	SetUser(ctx, "@chooblarin")
	seg.Close()

	got, err := td.Recv()
	if err != nil {
		t.Error(err)
	}
	want := &schema.Segment{
		Name:      "foobar",
		ID:        seg.id,
		TraceID:   seg.traceID,
		StartTime: 1000000000,
		EndTime:   1000000000,
		User:      "@chooblarin",
		Service:   ServiceData,
		AWS:       xrayData,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestSegment_AddAnnotation(t *testing.T) {
	nowFunc = fixedTime
	defer func() { nowFunc = time.Now }()

	ctx, td := NewTestDaemon(nil)
	defer td.Close()

	ctx, seg := BeginSegment(ctx, "foobar")
	AddAnnotationBool(ctx, "boolean", true)
	AddAnnotationInt64(ctx, "int64", -42)
	AddAnnotationUint64(ctx, "uint64", 42)
	AddAnnotationFloat64(ctx, "float64", 3.14)
	AddAnnotationString(ctx, "string", "@chooblarin")
	seg.Close()

	got, err := td.Recv()
	if err != nil {
		t.Error(err)
	}
	want := &schema.Segment{
		Name:      "foobar",
		ID:        seg.id,
		TraceID:   seg.traceID,
		StartTime: 1000000000,
		EndTime:   1000000000,
		Annotations: map[string]interface{}{
			"boolean": true,
			"int64":   -42.0,
			"uint64":  42.0,
			"float64": 3.14,
			"string":  "@chooblarin",
		},
		Service: ServiceData,
		AWS:     xrayData,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestDetachContextSegment(t *testing.T) {
	nowFunc = fixedTime
	defer func() { nowFunc = time.Now }()

	ctx, td := NewTestDaemon(nil)
	defer td.Close()

	ctx, cancel := context.WithCancel(ctx)
	ctx1, _ := BeginSegment(ctx, "foobar")
	ctx2 := DetachContextSegment(ctx1)
	cancel() // ctx1 is canceled.

	seg := ContextSegment(ctx2) // get segment from ctx2
	seg.Close()

	want := &schema.Segment{
		Name:      "foobar",
		ID:        seg.id,
		TraceID:   seg.traceID,
		StartTime: 1000000000,
		EndTime:   1000000000,
		Service:   ServiceData,
		AWS:       xrayData,
	}
	got, err := td.Recv()
	if err != nil {
		t.Error(err)
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}

	select {
	case <-ctx2.Done():
		t.Error(ctx2.Err())
	default:
		// ctx1 is canceled, but ctx2 is not.
	}
}

func BenchmarkBeginSegment(b *testing.B) {
	ctx, td := NewNullDaemon()
	defer td.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, seg := BeginSegment(ctx, "foobar")
		_ = ctx // do something using ctx
		seg.Close()
	}
}

func BenchmarkBeginSegmentParallel(b *testing.B) {
	ctx, td := NewNullDaemon()
	defer td.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx, seg := BeginSegment(ctx, "foobar")
			_ = ctx // do something using ctx
			seg.Close()
		}
	})
}
