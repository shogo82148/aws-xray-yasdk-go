package xray

import (
	"context"
	"testing"
)

func TestCapture(t *testing.T) {
	ctx, td := NewTestDaemon()
	defer td.Close()

	ctx, seg := BeginSegment(ctx, "root")
	err := Capture(ctx, "capture", func(ctx context.Context) error {
		return nil
	})
	seg.Close()
	if err != nil {
		t.Fatal(err)
	}

	s, err := td.Recv()
	if err != nil {
		t.Error(err)
	}
	if s.Name != "capture" {
		t.Errorf("want %q, got %q", "capture", s.Name)
	}

	s, err = td.Recv()
	if err != nil {
		t.Error(err)
	}
	if s.Name != "root" {
		t.Errorf("name: want %q, got %q", "root", s.Name)
	}
}
