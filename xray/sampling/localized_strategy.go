package sampling

import (
	crand "crypto/rand"
	"encoding/binary"
	"math/rand"
	"sync"
	"time"
)

// LocalizedStrategy makes trace sampling decisions based on
// a set of rules provided in a local JSON file. Trace sampling
// decisions are made by the root node in the trace. If a
// sampling decision is made by the root service, it will be passed
// to downstream services through the trace header.
type LocalizedStrategy struct {
	manifest         *Manifest
	reservoirs       []*reservoir
	defaultReservoir *reservoir
	mu               sync.Mutex
	randFunc         func() float64
}

// NewLocalizedStrategy returns new LocalizedStrategy.
func NewLocalizedStrategy(manifest *Manifest) (*LocalizedStrategy, error) {
	if manifest == nil {
		manifest = DefaultSamplingRule
	}
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	cp := manifest.Copy()
	cp.normalize()
	reservoirs := make([]*reservoir, 0, len(cp.Rules))
	for _, r := range cp.Rules {
		reservoirs = append(reservoirs, &reservoir{
			capacity: r.FixedTarget,
		})
	}
	defaultReservoir := &reservoir{
		capacity: cp.Default.FixedTarget,
	}
	return &LocalizedStrategy{
		manifest:         cp,
		reservoirs:       reservoirs,
		defaultReservoir: defaultReservoir,
	}, nil
}

// ShouldTrace implements Strategy.
func (s *LocalizedStrategy) ShouldTrace(req *Request) *Decision {
	for i, r := range s.manifest.Rules {
		if r.Match(req) {
			return s.sampling(s.reservoirs[i], r.Rate)
		}
	}
	return s.sampling(s.defaultReservoir, s.manifest.Default.Rate)
}

func (s *LocalizedStrategy) sampling(r *reservoir, rate float64) *Decision {
	if r.Take() {
		return &Decision{
			Sample: true,
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return &Decision{
		Sample: s.randLocked() < rate,
	}
}

// returns a pseudo-random number in [0.0,1.0). s.mu should be locked.
func (s *LocalizedStrategy) randLocked() float64 {
	if s.randFunc != nil {
		return s.randFunc()
	}
	// lazy initialize of random generator
	var seed int64
	if err := binary.Read(crand.Reader, binary.BigEndian, &seed); err != nil {
		// fallback to timestamp
		seed = time.Now().UnixNano()
	}
	s.randFunc = rand.New(rand.NewSource(seed)).Float64
	return s.randFunc()
}
