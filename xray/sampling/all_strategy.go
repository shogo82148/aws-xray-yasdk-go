package sampling

// allStrategy samples all segments.
func allStrategy(req *Request) *Decision {
	return &Decision{
		Sample: true,
	}
}

// NewAllStrategy returns the strategy that samples all segments.
func NewAllStrategy() Strategy {
	return StrategyFunc(allStrategy)
}
