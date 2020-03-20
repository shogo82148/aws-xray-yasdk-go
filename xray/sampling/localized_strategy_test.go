package sampling

import (
	"math"
	"testing"
)

var _ Strategy = (*LocalizedStrategy)(nil)

func TestLocalizedStrategy(t *testing.T) {
	s, err := NewLocalizedStrategy(&Manifest{
		Version: 2,
		Rules: []*Rule{
			{
				Host:        "*",
				URLPath:     "*",
				HTTPMethod:  "POST",
				Rate:        1,
				ServiceName: "*",
			},
		},
		Default: &Rule{
			Rate: 0.05,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var random float64
	s.randFunc = func() float64 { return random }
	testRate := func(req *Request, rate float64) {
		t.Helper()
		random = rate
		if sd := s.ShouldTrace(req); sd.Sample {
			t.Error("want false, got true")
		}
		random = math.Nextafter(rate, 0)
		if sd := s.ShouldTrace(req); !sd.Sample {
			t.Error("want true, got false")
		}
	}

	// it will match the first rule.
	req := &Request{
		Host:   "example.com",
		Method: "POST",
		URL:    "/",
	}
	testRate(req, 1.0)

	// fallback to the default rule
	req = &Request{
		Host:   "example.com",
		Method: "GET",
		URL:    "/",
	}
	testRate(req, 0.05)
}
