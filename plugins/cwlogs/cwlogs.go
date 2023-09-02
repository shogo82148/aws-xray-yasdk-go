// Package cwlogs provides a plugin for Amazon CloudWatch Logs.
// The plugin associates segments with log groups,
// and allows you to view the log of a trace using [CloudWatch ServiceLens].
//
// The following is an example for associating a log group named "/your-application/log-group-name".
//
//	plugin := cwlogs.New(&cwlogs.Config{
//	  LogReferences: []*schema.LogReference{
//	    { LogGroup: "/your-application/log-group-name" },
//	  },
//	})
//	xray.AddPlugin(plugin)
//
// And then, you need to add X-Ray Trace ID into your log.
//
// [CloudWatch ServiceLens]: https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/servicelens_service_map_traces.html
package cwlogs

import (
	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

// Config is configure of Amazon CloudWatch Logs plugin.
type Config struct {
	LogReferences []*schema.LogReference
}

type cwlogsPlugin struct {
	LogReferences []*schema.LogReference
}

// New creates new Amazon CloudWatch Logs plugin.
func New(config *Config) xray.Plugin {
	return cwlogsPlugin{
		LogReferences: config.LogReferences,
	}
}

// HandleSegment implements xray.Plugin.
func (p cwlogsPlugin) HandleSegment(seg *xray.Segment, doc *schema.Segment) {
	if doc.AWS == nil {
		doc.AWS = schema.AWS{}
	}
	doc.AWS.AddLogReferences(p.LogReferences)
}

// Origin implements xray.Plugin.
func (cwlogsPlugin) Origin() string { return "" }
