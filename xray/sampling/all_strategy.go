package sampling

// allStrategy samples all segments.
type allStrategy struct{}

// NewAllStrategy returns the strategy that samples all segments.
func NewAllStrategy() Strategy {
	return &allStrategy{}
}

// ShouldTrace implements Strategy.
func (s *allStrategy) ShouldTrace(req *Request) *Decision {
	return &Decision{
		Sample: true,
	}
}
