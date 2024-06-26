// Package ecs provides a plugin for Amazon ECS (Amazon Elastic Container Service).
// The plugin collects the information of ECS containers, and record them.
// The container ID, the container name and the container ARN are available.
//
// To enable this plugin, please import the ecs/init package.
//
//	import _ "github.com/shogo82148/aws-xray-yasdk-go/xray/plugins/ecs/init"
//
// or if you want to load conditionally at runtime, use Init() function.
//
//	import _ "github.com/shogo82148/aws-xray-yasdk-go/xray/plugins/ecs"
//	ecs.Init()
package ecs

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/shogo82148/aws-xray-yasdk-go/internal/envconfig"
	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
	"github.com/shogo82148/go-retry/v2"
)

const cgroupPath = "/proc/self/cgroup"

type plugin struct {
	ECS           *schema.ECS
	logReferences []*schema.LogReference
}

var once sync.Once

// Init activates ECS Plugin at runtime.
func Init() {
	once.Do(initECSPlugin)
}

func initECSPlugin() {
	if runtime.GOOS != "linux" {
		return
	}
	client := newMetadataFetcher()
	if client == nil {
		// not in ECS Container, skip installing the plugin
		return
	}
	// we don't reuse the client, so release its resources.
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	meta, err := client.Fetch(ctx)
	if err != nil {
		return
	}
	hostname, err := os.Hostname()
	if err != nil {
		return
	}
	xray.AddPlugin(&plugin{
		ECS: &schema.ECS{
			Container:    hostname,
			ContainerID:  containerID(cgroupPath),
			ContainerArn: meta.ContainerARN,
		},
		logReferences: meta.LogReferences(),
	})
}

// HandleSegment implements Plugin.
func (p *plugin) HandleSegment(seg *xray.Segment, doc *schema.Segment) {
	if doc.AWS == nil {
		doc.AWS = schema.AWS{}
	}
	doc.AWS.SetECS(p.ECS)
	doc.AWS.AddLogReferences(p.logReferences)
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

type containerMetadata struct {
	ContainerARN string
	LogDriver    string
	LogOptions   *logOptions

	// we don't use other fields
}

type logOptions struct {
	AWSLogsGroup  string `json:"awslogs-group"`
	AWSLogsRegion string `json:"awslogs-region"`
}

func (meta *containerMetadata) AccountID() string {
	arn := meta.ContainerARN

	// trim "arn:aws:ecs:${AWS::Region}:"
	for i := 0; i < 4; i++ {
		idx := strings.IndexByte(arn, ':')
		if idx < 0 {
			return ""
		}
		arn = arn[idx+1:]
	}

	idx := strings.IndexByte(arn, ':')
	if idx < 0 {
		return ""
	}
	return arn[:idx]
}

func (meta *containerMetadata) LogReferences() []*schema.LogReference {
	opt := meta.LogOptions
	if opt == nil || opt.AWSLogsGroup == "" {
		return nil
	}

	accountID := meta.AccountID()
	var arn string
	if opt.AWSLogsRegion != "" && accountID != "" {
		arn = "arn:aws:logs:" + opt.AWSLogsRegion + ":" + accountID + ":log-group:" + opt.AWSLogsGroup
	}

	return []*schema.LogReference{
		{
			LogGroup: opt.AWSLogsGroup,
			ARN:      arn,
		},
	}
}

type metadataFetcher struct {
	client  *http.Client
	url     string
	timeout time.Duration
	policy  *retry.Policy
}

func newMetadataFetcher() *metadataFetcher {
	url := os.Getenv("ECS_CONTAINER_METADATA_URI_V4")
	if url == "" {
		// fallback to v3 endpoint
		url = os.Getenv("ECS_CONTAINER_METADATA_URI")
	}
	if !strings.HasPrefix(url, "http://") {
		return nil
	}
	timeout := envconfig.MetadataServiceTimeout()
	client := &http.Client{
		Transport: &http.Transport{
			// ignore proxy configure from the environment values
			Proxy: nil,

			// metadata endpoint is in same network,
			// so timeout can be shorter.
			DialContext: (&net.Dialer{
				Timeout:   timeout,
				KeepAlive: timeout,
				DualStack: true,
			}).DialContext,
			MaxIdleConns:          5,
			IdleConnTimeout:       timeout,
			TLSHandshakeTimeout:   timeout,
			ExpectContinueTimeout: timeout,
		},
	}
	return &metadataFetcher{
		client:  client,
		url:     url,
		timeout: timeout,
		policy: &retry.Policy{
			MaxCount: envconfig.MetadataServiceNumAttempts(),
		},
	}
}

func (c *metadataFetcher) Fetch(ctx context.Context) (*containerMetadata, error) {
	return retry.DoValue(ctx, c.policy, func() (*containerMetadata, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
		if err != nil {
			return nil, err
		}

		resp, err := c.client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		var data containerMetadata
		dec := json.NewDecoder(resp.Body)
		if err := dec.Decode(&data); err != nil {
			return nil, err
		}
		return &data, nil
	})
}

func (c *metadataFetcher) Close() {
	c.client.CloseIdleConnections()
}
