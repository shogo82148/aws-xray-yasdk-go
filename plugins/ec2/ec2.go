package ec2

import (
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

type ec2plugin struct {
	EC2 *schema.EC2
}

// Init activates EC2Plugin at runtime.
func Init() {
	session, err := session.NewSession()
	if err != nil {
		return
	}
	client := ec2metadata.New(session)
	doc, err := client.GetInstanceIdentityDocument()
	if err != nil {
		return
	}
	xray.AddPlugin(&ec2plugin{
		EC2: &schema.EC2{
			InstanceID: doc.InstanceID,
			AvailabilityZone: doc.AvailabilityZone,
		},
	})
}

// HandleSegment implements Plugin.
func (p *ec2plugin) HandleSegment(seg *Segment, doc *schema.Segment) {
	if doc.AWS == nil {
		doc.AWS = &schema.AWS{}
	}
	doc.AWS.EC2 = p.EC2
}

// Origin implements Plugin.
func (xrayPlugin) Origin() string { return schema.OriginEC2Instance }
