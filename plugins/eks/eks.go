package eks

import (
	"os"
	"runtime"

	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

const (
	caCertificateFile = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	tokenFile         = "/var/run/secrets/kubernetes.io/serviceaccount/token"
)

type plugin struct {
	EKS *schema.EKS
}

// Init activates EKS Plugin at runtime.
func Init() {
	if runtime.GOOS != "linux" {
		return
	}
	if _, err := os.Stat(tokenFile); err != nil {
		return
	}
	hostname, err := os.Hostname()
	if err != nil {
		return
	}
	xray.AddPlugin(&plugin{
		EKS: &schema.EKS{
			ClusterName: "", // TODO
			ContainerID: "", // TODO
			Pod:         hostname,
		},
	})
}

// HandleSegment implements Plugin.
func (p *plugin) HandleSegment(seg *xray.Segment, doc *schema.Segment) {
	if doc.AWS == nil {
		doc.AWS = schema.AWS{}
	}
	doc.AWS.SetEKS(p.EKS)
}

// Origin implements Plugin.
func (*plugin) Origin() string { return schema.OriginEKSContainer }
