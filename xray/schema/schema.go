// Package schema is a utils for generating AWS X-Ray Segment Documents.
// ref. https://docs.aws.amazon.com/xray/latest/devguide/xray-api-segmentdocuments.html
package schema

// Segment is a segment
type Segment struct {
	// Required

	// The logical name of the service that handled the request, up to 200 characters.
	// For example, your application's name or domain name.
	Name string `json:"name"`

	// ID is a 64-bit identifier for the segment,
	// unique among segments in the same trace, in 16 hexadecimal digits.
	ID string `json:"id"`

	// TraceID is a unique identifier that connects all segments and subsegments originating
	// from a single client request. Trace ID Format.
	TraceID string `json:"trace_id"`

	// StartTime is a number that is the time the segment was created,
	// in floating point seconds in epoch time.
	StartTime float64 `json:"start_time"`

	// EndTime is a number that is the time the segment was closed.
	EndTime float64 `json:"end_time,omitempty"`

	// InProgress is a boolean, set to true instead of specifying an end_time to record that a segment is started, but is not complete.
	InProgress bool `json:"in_progress,omitempty"`
}
