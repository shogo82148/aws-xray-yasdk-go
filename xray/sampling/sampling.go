package sampling

// Decision contains sampling decision and the rule matched for an incoming request
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
	ShouldTrace(request *Request) *Decision
}
