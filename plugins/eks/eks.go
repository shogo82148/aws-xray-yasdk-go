package eks

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/shogo82148/aws-xray-yasdk-go/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/xraylog"
)

const (
	caCertificateFile = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	tokenFile         = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	cgroupPath        = "/proc/self/cgroup"
)

type plugin struct {
	EKS           *schema.EKS
	logReferences []*schema.LogReference
}

// Init activates EKS Plugin at runtime.
func Init() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if runtime.GOOS != "linux" {
		return
	}
	caCert, err := ioutil.ReadFile(caCertificateFile)
	if err != nil {
		// it seems not to be in kubernetes environment.
		// just ignore error.
		xraylog.Debugf(ctx, "failed to read ca.crt: %v", err)
		return
	}
	token, err := ioutil.ReadFile(tokenFile)
	if err != nil {
		xraylog.Debugf(ctx, "failed to read token: %v", err)
		return
	}

	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(caCert)
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
				DualStack: true,
			}).DialContext,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			TLSClientConfig: &tls.Config{
				RootCAs: certPool,
			},
		},
	}
	// we don't reuse the client, so release its resources.
	defer closeIdleConnections(client)

	hostname, err := os.Hostname()
	if err != nil {
		xraylog.Debugf(ctx, "failed to get hostname: %v", err)
		return
	}
	clusterName := clusterName(ctx, client, string(bytes.TrimSpace(token)))
	containerID := containerID(ctx, cgroupPath)
	xray.AddPlugin(&plugin{
		EKS: &schema.EKS{
			ClusterName: clusterName,
			ContainerID: containerID,
			Pod:         hostname,
		},
		logReferences: []*schema.LogReference{
			{
				LogGroup: "/aws/containerinsights/" + clusterName + "/application",
			},
		},
	})
}

// HandleSegment implements Plugin.
func (p *plugin) HandleSegment(seg *xray.Segment, doc *schema.Segment) {
	if doc.AWS == nil {
		doc.AWS = schema.AWS{}
	}
	doc.AWS.SetEKS(p.EKS)
	doc.AWS.AddLogReferences(p.logReferences)
}

// Origin implements Plugin.
func (*plugin) Origin() string { return schema.OriginEKSContainer }

// Reads the docker-generated cgroup file that lists the full (untruncated) docker container ID at the end of each line.
// This method takes advantage of that fact by just reading the 64-character ID from the end of the first line.
func containerID(ctx context.Context, cgroup string) string {
	const idLength = 64
	f, err := os.Open(cgroup)
	if err != nil {
		xraylog.Debugf(ctx, "failed to read cgroup: %v", err)
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			xraylog.Debugf(ctx, "failed to read cgroup: %v", err)
		}
		return ""
	}
	line := scanner.Text()
	if len(line) < idLength {
		return ""
	}
	line = line[len(line)-idLength:]
	xraylog.Debugf(ctx, "container id is %s", line)
	return line
}

func clusterName(ctx context.Context, client *http.Client, token string) string {
	const apiEndpoint = "https://kubernetes.default.svc"
	const configMapPath = "/api/v1/namespaces/amazon-cloudwatch/configmaps/cluster-info"
	req, err := http.NewRequest(http.MethodGet, apiEndpoint+configMapPath, nil)
	if err != nil {
		xraylog.Debugf(ctx, "failed to create a new request: %v", err)
		return ""
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := client.Do(req)
	if err != nil {
		xraylog.Debugf(ctx, "failed to get the cluster name: %v", err)
		return ""
	}
	defer resp.Body.Close()

	var data struct {
		Data struct {
			ClusterName string `json:"cluster.name"`
		} `json:"data"`
	}
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&data); err != nil {
		xraylog.Debugf(ctx, "failed to decode: %v", err)
		return ""
	}
	xraylog.Debugf(ctx, "cluster name is %s", data.Data.ClusterName)
	return data.Data.ClusterName
}

// call CloseIdleConnections() if the client have the method.
// for Go 1.11
func closeIdleConnections(client interface{}) {
	type IdleConnectionsCloser interface {
		CloseIdleConnections()
	}
	if c, ok := client.(IdleConnectionsCloser); ok {
		c.CloseIdleConnections()
	}
}
