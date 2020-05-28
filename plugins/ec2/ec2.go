package ec2

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"time"

	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
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

type ec2plugin struct {
	EC2 *schema.EC2
}

// Init activates EC2Plugin at runtime.
func Init() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	c := &client{
		base: "http://169.254.169.254",
	}
	doc, err := c.getInstanceIdentityDocument(ctx)
	if err != nil {
		return
	}
	xray.AddPlugin(&ec2plugin{
		EC2: &schema.EC2{
			InstanceID:       doc.InstanceID,
			AvailabilityZone: doc.AvailabilityZone,
		},
	})
}

// HandleSegment implements Plugin.
func (p *ec2plugin) HandleSegment(seg *xray.Segment, doc *schema.Segment) {
	if doc.AWS == nil {
		doc.AWS = schema.AWS{}
	}
	doc.AWS.SetEC2(p.EC2)
}

// Origin implements Plugin.
func (*ec2plugin) Origin() string { return schema.OriginEC2Instance }
