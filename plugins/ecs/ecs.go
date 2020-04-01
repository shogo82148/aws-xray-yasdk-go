package ecs

import (
	"bufio"
	"os"
	"runtime"
	"strings"

	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

const cgroupPath = "/proc/self/cgroup"

type plugin struct {
	ECS *schema.ECS
}

// Init activates ECS Plugin at runtime.
func Init() {
	if runtime.GOOS != "linux" {
		return
	}
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
			Container:   hostname,
			ContainerID: containerID(cgroupPath),
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
