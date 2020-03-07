package xray

import (
	"regexp"
	"testing"
)

func TestNewTraceID(t *testing.T) {
	id := NewTraceID()
	pattern := `^1-[0-9a-fA-F]{8}-[0-9a-fA-F]{24}$`
	if matched, err := regexp.MatchString(pattern, id); err != nil || !matched {
		t.Errorf("id should match %q, but got %q", pattern, id)
	}
}

func TestNewSegmentID(t *testing.T) {
	id := NewSegmentID()
	pattern := `^[0-9a-fA-F]{16}$`
	if matched, err := regexp.MatchString(pattern, id); err != nil || !matched {
		t.Errorf("id should match %q, but got %q", pattern, id)
	}
}

func TestBeginSegment(t *testing.T) {
	ctx, td := NewTestDaemon()
	defer td.Close()

	ctx, seg := BeginSegment(ctx, "foobar")
	_ = ctx // do something using ctx
	seg.Close()

	s, err := td.Recv()
	if err != nil {
		t.Error(err)
	}
	if s.Name != "foobar" {
		t.Errorf("name: want %q, got %q", "foobar", s.Name)
	}
}

func TestBeginSubsegment(t *testing.T) {
	ctx, td := NewTestDaemon()
	defer td.Close()

	ctx, root := BeginSegment(ctx, "root")
	ctx, seg := BeginSubsegment(ctx, "subsegment")
	_ = ctx // do something using ctx
	seg.Close()
	root.Close()

	// we will receive Independent Subsegment
	s, err := td.Recv()
	if err != nil {
		t.Error(err)
	}
	if s.Name != "subsegment" {
		t.Errorf("name: want %q, got %q", "subsegment", s.Name)
	}
	if s.Type != "subsegment" {
		t.Errorf("name: want %q, got %q", "subsegment", s.Type)
	}
	if s.ParentID != root.id {
		t.Errorf("want parent id is %q, got %q", root.id, s.ParentID)
	}
	if s.TraceID != root.traceID {
		t.Errorf("want trace id is %q, got %q", root.traceID, s.TraceID)
	}

	s, err = td.Recv()
	if err != nil {
		t.Error(err)
	}
	if s.Name != "root" {
		t.Errorf("name: want %q, got %q", "root", s.Name)
	}
}
