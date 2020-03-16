package sampling

// LocalizedStrategy makes trace sampling decisions based on
// a set of rules provided in a local JSON file. Trace sampling
// decisions are made by the root node in the trace. If a
// sampling decision is made by the root service, it will be passed
// to downstream services through the trace header.
type LocalizedStrategy struct {
	manifest         *Manifest
	reservoirs       []reservoir
	defaultReservoir reservoir
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
	return &LocalizedStrategy{
		manifest:   cp,
		reservoirs: make([]reservoir, len(cp.Rules)),
	}, nil
}

// ShouldTrace implements Strategy.
func (s *LocalizedStrategy) ShouldTrace(req *Request) *Decision {
	for i, r := range s.manifest.Rules {
		if r.Match(req) {
			return s.sampling(&s.reservoirs[i], r.Rate)
		}
	}
	return s.sampling(&s.defaultReservoir, s.manifest.Default.Rate)
}

func (s *LocalizedStrategy) sampling(r *reservoir, rate float64) *Decision {
	if r.Take() {
		return &Decision{
			Sample: true,
		}
	}
	return &Decision{
		Sample: true,
	}
}
