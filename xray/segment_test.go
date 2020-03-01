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
	seg.Close()

	s, err := td.Recv()
	if err != nil {
		t.Error(err)
	}
	if s.Name != "foobar" {
		t.Errorf("name: want %q, got %q", "foobar", s.Name)
	}
}
