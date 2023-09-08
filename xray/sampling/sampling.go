// Package sampling provides the sampling strategy.
package sampling

// Decision contains sampling decision and the rule matched for an incoming request.
type Decision struct {
	Sample bool
	Rule   *string
}

// Request represents parameters used to make a sampling decision.
type Request struct {
	Host        string
	Method      string
	URL         string
	ServiceName string
	ServiceType string
}

// Strategy provides an interface for implementing trace sampling strategies.
type Strategy interface {
	// ShouldTrace returns a sampling decision for an incoming request.
	ShouldTrace(request *Request) *Decision
}

// StrategyFunc is an adapter to allow the use of ordinary functions as sampling strategies.
type StrategyFunc func(request *Request) *Decision

func (s StrategyFunc) ShouldTrace(request *Request) *Decision {
	return s(request)
}
