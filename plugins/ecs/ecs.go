package ecs

import (
	"os"
	"strings"

	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

type plugin struct {
	ECS *schema.ECS
}

// Init activates ECS Plugin at runtime.
func Init() {
	uri := os.Getenv("ECS_CONTAINER_METADATA_URI")
	if !strings.HasPrefix(uri, "http://") {
		return
	}
	hostname, err := os.Hostname()
	if err != nil {
		return
	}
	xray.AddPlugin(&plugin{
		ECS: &schema.ECS{
			Container: hostname,
		},
	})
}

// HandleSegment implements Plugin.
func (p *plugin) HandleSegment(seg *xray.Segment, doc *schema.Segment) {
	if doc.AWS == nil {
		doc.AWS = schema.AWS{}
	}
	doc.AWS.SetECS(p.ECS)
}

// Origin implements Plugin.
func (*plugin) Origin() string { return schema.OriginECSContainer }
