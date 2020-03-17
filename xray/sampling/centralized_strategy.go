package sampling

import (
	xraySvc "github.com/aws/aws-sdk-go/service/xray"
)

// CentralizedStrategy is an implementation of SamplingStrategy.
type CentralizedStrategy struct {
	fallback *LocalizedStrategy
	xray     *xraySvc.XRay
}

// NewCentralizedStrategy returns new centralized sampling strategy with a fallback on
// the local rule. If local rule is nil, the DefaultSamplingRule is used.
func NewCentralizedStrategy(addr string, manifest *Manifest) (*CentralizedStrategy, error) {
	local, err := NewLocalizedStrategy(manifest)
	if err != nil {
		return nil, err
	}
	return &CentralizedStrategy{
		fallback: local,
	}, nil
}

// Close stops polling.
func (s *CentralizedStrategy) Close() {

}

// ShouldTrace implements Strategy.
func (s *CentralizedStrategy) ShouldTrace(req *Request) *Decision {
	// TODO: @shogo82148 implement me!
	return s.fallback.ShouldTrace(req)
}
