package sampling

import (
	"testing"
	"time"
)

func TestReservoir(t *testing.T) {
	now := int64(1000000000)
	r := &reservoir{
		nowFunc: func() time.Time {
			return time.Unix(now, 0)
		},
		capacity: 5,
	}

	for i := 0; i < 5; i++ {
		if !r.Take() {
			t.Errorf("want true, got false")
		}
	}
	// capacity over, so Take returns false
	if r.Take() {
		t.Errorf("want true, got false")
	}

	// reset counter in next second
	now++

	for i := 0; i < 5; i++ {
		if !r.Take() {
			t.Errorf("want true, got false")
		}
	}
	// capacity over, so Take returns false
	if r.Take() {
		t.Errorf("want true, got false")
	}
}
