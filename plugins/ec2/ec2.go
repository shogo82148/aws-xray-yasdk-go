// Package ec2 provides a plugin for Amazon Elastic Compute Cloud.
// The plugin collects the information of EC2 instances, and record them.
// The instance ID, the availability zone, the instance type, and the AMI ID are available.
//
// If CloudWatch Agent is installed in the instance, the plugin collects the CloudWatch Logs Groups.
// It allows you to view the log of a trace using CloudWatch ServiceLens.
//
// To enable this plugin, please import the ec2/init package.
//
//     import _ "github.com/shogo82148/aws-xray-yasdk-go/xray/plugins/ec2/init"
//
// or if you want to load conditionally at runtime, use Init() function.
//
//     import _ "github.com/shogo82148/aws-xray-yasdk-go/xray/plugins/ec2"
//     ec2.Init()
//
package ec2

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/xraylog"
)

type ec2InstanceIdentityDocument struct {
	MarketplaceProductCodes []string  `json:"marketplaceProductCodes"`
	AvailabilityZone        string    `json:"availabilityZone"`
	PrivateIP               string    `json:"privateIp"`
	Version                 string    `json:"version"`
	Region                  string    `json:"region"`
	InstanceID              string    `json:"instanceId"`
	BillingProducts         []string  `json:"billingProducts"`
	InstanceType            string    `json:"instanceType"`
	AccountID               string    `json:"accountId"`
	PendingTime             time.Time `json:"pendingTime"`
	ImageID                 string    `json:"imageId"`
	KernelID                string    `json:"kernelId"`
	RamdiskID               string    `json:"ramdiskId"`
	Architecture            string    `json:"architecture"`
}

var httpClient = &http.Client{
	Transport: &http.Transport{
		Proxy: nil, // ignore proxy configure from the environment values
		DialContext: (&net.Dialer{
			Timeout:   time.Second,
			KeepAlive: time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          5,
		IdleConnTimeout:       time.Second,
		TLSHandshakeTimeout:   time.Second,
		ExpectContinueTimeout: time.Second,
	},
	Timeout: time.Second,
}

type client struct {
	// base url for the instance metadata api
	// typically it is http://169.254.169.254
	base string

	// api token for IMDSv2
	token string

	// TTL for token
	ttl time.Time
}

func (c *client) refreshToken(ctx context.Context) error {
	now := time.Now()
	if c.token != "" && c.ttl.After(now) {
		// no need to refresh
		return nil
	}

	req, err := http.NewRequest(http.MethodPut, c.base+"/latest/api/token", nil)
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)
	req.Header.Set("x-aws-ec2-metadata-token-ttl-seconds", "10")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// IMDSv2 may be disabled; fallback to IMDSv1.
		return nil
	}

	token, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	c.token = string(token)
	c.ttl = now.Add(5 * time.Second)

	return nil
}

func (c *client) getInstanceIdentityDocument(ctx context.Context) (*ec2InstanceIdentityDocument, error) {
	if err := c.refreshToken(ctx); err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodGet, c.base+"/latest/dynamic/instance-identity/document", nil)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	if c.token != "" {
		req.Header.Set("x-aws-ec2-metadata-token", c.token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var doc ec2InstanceIdentityDocument
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// Find logging configure of CloudWatch Agent.
// https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/CloudWatch-Agent-Configuration-File-Details.html#CloudWatch-Agent-Configuration-File-Logssection
func getLogReference(ctx context.Context) []*schema.LogReference {
	var path string
	if programData := os.Getenv("ProgramData"); programData != "" {
		// Windows
		path = filepath.Join(programData, "Amazon", "AmazonCloudWatchAgent", "log-config.json")
	} else if filepath.Separator == '/' {
		// Linux
		path = "/opt/aws/amazon-cloudwatch-agent/etc/log-config.json"
	} else {
		// Unknown platform
		return nil
	}
	return parseAgentConfig(ctx, path)
}

func parseAgentConfig(ctx context.Context, path string) []*schema.LogReference {
	xraylog.Debugf(ctx, "plugin/ec2: loading cloudwatch agent configure file: %s", path)

	f, err := os.Open(path)
	if err != nil {
		xraylog.Debugf(ctx, "plugin/ec2: fail to open configure file: %v", err)
		return nil
	}
	defer f.Close()

	var v interface{}
	dec := json.NewDecoder(f)
	if err := dec.Decode(&v); err != nil {
		xraylog.Debugf(ctx, "plugin/ec2: fail to parse configure file: %v", err)
		return nil
	}

	var w jsonWalker
	w.Walk(v)
	sort.Strings(w.logs)

	logs := make([]*schema.LogReference, 0, len(w.logs))
	for _, v := range w.logs {
		logs = append(logs, &schema.LogReference{LogGroup: v})
	}
	return logs
}

type jsonWalker struct {
	logs []string
}

func (w *jsonWalker) Walk(v interface{}) {
	switch v := v.(type) {
	case map[string]interface{}:
		for key, value := range v {
			if key == "log_group_name" {
				// collect all { "log_group_name": "string value" }
				if s, ok := value.(string); ok {
					w.logs = appendIfNotExists(w.logs, s)
					continue
				}
			}
			w.Walk(value)
		}
	case []interface{}:
		for _, value := range v {
			w.Walk(value)
		}
	}
}

func appendIfNotExists(slice []string, s string) []string {
	for _, v := range slice {
		if v == s {
			// s is already in slice
			// no need to append it
			return slice
		}
	}
	return append(slice, s)
}

type ec2plugin struct {
	EC2           *schema.EC2
	logReferences []*schema.LogReference
}

// Init activates EC2Plugin at runtime.
func Init() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	disabled := os.Getenv("AWS_EC2_METADATA_DISABLED")
	if strings.EqualFold(disabled, "true") {
		xraylog.Debugf(ctx, "plugin/ec2: imds is disabled by the environment value")
		return
	}

	base := os.Getenv("AWS_EC2_METADATA_SERVICE_ENDPOINT")
	if base == "" {
		mode := os.Getenv("AWS_EC2_METADATA_SERVICE_ENDPOINT_MODE")
		switch {
		case mode == "" || strings.EqualFold(mode, "IPv4"):
			base = "http://169.254.169.254"
		case strings.EqualFold(mode, "IPv6"):
			base = "http://[fd00:ec2::254]"
		default:
			xraylog.Debugf(ctx, "plugin/ec2: unknown aws ec2 metadata service endpoint mode: %q", mode)
			return
		}
	}
	base = strings.TrimSuffix(base, "/")
	c := &client{
		base: base,
	}
	doc, err := c.getInstanceIdentityDocument(ctx)
	if err != nil {
		xraylog.Debugf(ctx, "plugin/ec2: failed to get identity document: %v", err)
		return
	}
	xray.AddPlugin(&ec2plugin{
		EC2: &schema.EC2{
			InstanceID:       doc.InstanceID,
			AvailabilityZone: doc.AvailabilityZone,
			InstanceSize:     doc.InstanceType,
			AMIID:            doc.ImageID,
		},
		logReferences: getLogReference(ctx),
	})
}

// HandleSegment implements Plugin.
func (p *ec2plugin) HandleSegment(seg *xray.Segment, doc *schema.Segment) {
	if doc.AWS == nil {
		doc.AWS = schema.AWS{}
	}
	doc.AWS.SetEC2(p.EC2)
	doc.AWS.AddLogReferences(p.logReferences)
}

// Origin implements Plugin.
func (*ec2plugin) Origin() string { return schema.OriginEC2Instance }
