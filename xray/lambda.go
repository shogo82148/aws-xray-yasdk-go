package xray

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"
)

const (
	lambdaInitializedDir  = "/tmp/.aws-xray"
	lambdaInitializedFile = "initialized"
	lambdaContextKey      = "x-amzn-trace-id"
)

func init() {
	if os.Getenv("LAMBDA_TASK_ROOT") == "" {
		return
	}
	err := os.MkdirAll(lambdaInitializedDir, 0755)
	if err != nil {
		log.Printf("failed to create %s: %v", lambdaInitializedDir, err)
		return
	}
	name := filepath.Join(lambdaInitializedDir, lambdaInitializedFile)
	f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("failed to create %s: %v", name, err)
		return
	}
	f.Close()

	now := time.Now()
	if err := os.Chtimes(name, now, now); err != nil {
		log.Printf("failed to change times of %s: %v", name, err)
		return
	}
}

func beginSubsegmentForLambda(ctx context.Context, header, name string) (context.Context, *Segment) {
	h := ParseTraceHeader(header)
	h.SamplingDecision = SamplingDecisionSampled
	if h.TraceID == "" {
		h.TraceID = NewTraceID()
	}

	seg := &Segment{
		ctx:           ctx,
		name:          name, // TODO: @shogo82148 sanitize the name
		id:            NewSegmentID(),
		startTime:     nowFunc(),
		totalSegments: 1,
		sampled:       true,
		traceID:       h.TraceID,
		traceHeader:   h,
	}
	seg.root = seg
	ctx = context.WithValue(ctx, segmentContextKey, seg)
	return ctx, seg
}
