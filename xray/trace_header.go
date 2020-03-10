package xray

import (
	"sort"
	"strings"
)

// TraceIDHeaderKey is the HTTP header name used for tracing.
const TraceIDHeaderKey string = "x-amzn-trace-id"

// SamplingDecision is whether or not the current segment has been sampled.
type SamplingDecision rune

const (
	// SamplingDecisionSampled indicates the current segment has been
	// sampled and will be sent to the X-Ray daemon.
	SamplingDecisionSampled SamplingDecision = '1'

	// SamplingDecisionNotSampled indicates the current segment has
	// not been sampled.
	SamplingDecisionNotSampled SamplingDecision = '0'

	// SamplingDecisionRequested indicates sampling decision will be
	// made by the downstream service and propagated
	// back upstream in the response.
	SamplingDecisionRequested SamplingDecision = '?'

	// SamplingDecisionUnknown indicates no sampling decision will be made.
	SamplingDecisionUnknown SamplingDecision = 0
)

// TraceHeader is the value of X-Amzn-Trace-Id.
type TraceHeader struct {
	TraceID          string
	ParentID         string
	SamplingDecision SamplingDecision

	AdditionalData map[string]string
}

// ParseTraceHeader parses X-Amzn-Trace-Id header.
func ParseTraceHeader(s string) TraceHeader {
	var header TraceHeader
	s = strings.TrimSpace(s)
	for _, kv := range strings.Split(s, ";") {
		idx := strings.IndexByte(kv, '=')
		if idx < 0 {
			// ignore invalid parameter
			continue
		}
		key := kv[:idx]
		value := kv[idx+1:]
		switch {
		case strings.EqualFold(key, "Root"):
			header.TraceID = strings.ToLower(value)
		case strings.EqualFold(key, "Parent"):
			header.ParentID = strings.ToLower(value)
		case strings.EqualFold(key, "Sampled"):
			switch value {
			case "1":
				header.SamplingDecision = SamplingDecisionSampled
			case "0":
				header.SamplingDecision = SamplingDecisionNotSampled
			case "?":
				header.SamplingDecision = SamplingDecisionRequested
			}
		case strings.EqualFold(key, "Self"):
			// Ignore any "Self=" trace ids injected from ALB.
		default:
			if header.AdditionalData == nil {
				header.AdditionalData = map[string]string{}
			}
			header.AdditionalData[key] = value
		}
	}
	return header
}

func (h TraceHeader) String() string {
	var b strings.Builder
	b.Grow(len("Root=1-5e645f3e-1dfad076a177c5ccc5de12f5;Parent=03babb4ba280be51;Sampled=1;"))
	if h.TraceID != "" {
		b.WriteString("Root=")
		b.WriteString(h.TraceID)
		b.WriteString(";")
	}
	if h.ParentID != "" {
		b.WriteString("Parent=")
		b.WriteString(h.ParentID)
		b.WriteString(";")
	}
	if h.SamplingDecision != SamplingDecisionUnknown {
		b.WriteString("Sampled=")
		b.WriteRune(rune(h.SamplingDecision))
		b.WriteString(";")
	}
	if len(h.AdditionalData) > 0 {
		keys := make([]string, 0, len(h.AdditionalData))
		var n int
		for key, value := range h.AdditionalData {
			keys = append(keys, key)
			n += len(key) + len(value) + 2 // '=' and ';'
		}
		b.Grow(n)
		sort.Strings(keys)
		for _, key := range keys {
			b.WriteString(key)
			b.WriteString("=")
			b.WriteString(h.AdditionalData[key])
			b.WriteString(";")
		}
	}
	s := b.String()
	if len(s) > 0 {
		// remove ';'
		s = s[:len(s)-1]
	}
	return s
}
