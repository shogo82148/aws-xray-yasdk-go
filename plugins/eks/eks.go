package eks

import (
	"bufio"
	"os"
	"runtime"

	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

const (
	caCertificateFile = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	tokenFile         = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	cgroupPath        = "/proc/self/cgroup"
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
			ContainerID: containerID(cgroupPath),
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

// Reads the docker-generated cgroup file that lists the full (untruncated) docker container ID at the end of each line.
// This method takes advantage of that fact by just reading the 64-character ID from the end of the first line.
func containerID(cgroup string) string {
	const idLength = 64
	f, err := os.Open(cgroup)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return ""
	}
	line := scanner.Text()
	if len(line) < idLength {
		return ""
	}
	return line[len(line)-idLength:]
}
