package xray

import (
	"context"
)

// Capture traces the provided synchronous function by
// beginning and closing a subsegment around its execution.
func Capture(ctx context.Context, name string, f func(context.Context) error) error {
	ctx, seg := BeginSubsegment(ctx, name)
	defer seg.Close()
	err := f(ctx)
	seg.AddError(err)
	return err
}
