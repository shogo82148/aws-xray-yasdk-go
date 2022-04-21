package ecs

import (
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestContainerID(t *testing.T) {
	tmp, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	want := "42e85902377f5b9e758dfa6537377e2da86338b4b40c20d875251082e8a1da84"
	dummyCGroup := filepath.Join(tmp, "tmpfile")
	if err := ioutil.WriteFile(dummyCGroup, []byte("14:name=systemd:/docker/"+want+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	got := containerID(dummyCGroup)
	if got != want {
		t.Errorf("want %s, got %s", want, got)
	}
}

func TestMetadataFetcher(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.WriteString(w, `{
			"DockerId": "7de2c0ca988f4162be5606783ffd0f6c-607325679",
			"Name": "example",
			"DockerName": "example",
			"Image": "123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/example:latest",
			"ImageID": "sha256:519d5922ec67f6a999740104362cadad3256011c2a39a59ad628b6727936807a",
			"Labels": {
			  "com.amazonaws.ecs.cluster": "arn:aws:ecs:ap-northeast-1:123456789012:cluster/example",
			  "com.amazonaws.ecs.container-name": "example",
			  "com.amazonaws.ecs.task-arn": "arn:aws:ecs:ap-northeast-1:123456789012:task/example/7de2c0ca988f4162be5606783ffd0f6c",
			  "com.amazonaws.ecs.task-definition-family": "example",
			  "com.amazonaws.ecs.task-definition-version": "9"
			},
			"DesiredStatus": "RUNNING",
			"KnownStatus": "RUNNING",
			"Limits": {
			  "CPU": 2
			},
			"CreatedAt": "2022-02-28T08:36:52.148607764Z",
			"StartedAt": "2022-02-28T08:36:52.148607764Z",
			"Type": "NORMAL",
			"Networks": [
			  {
				"NetworkMode": "awsvpc",
				"IPv4Addresses": [
				  "10.0.130.95"
				],
				"AttachmentIndex": 0,
				"MACAddress": "00:00:00:00:00:00",
				"IPv4SubnetCIDRBlock": "10.0.130.0/24",
				"DomainNameServers": [
				  "10.0.0.2"
				],
				"DomainNameSearchList": [
				  "ap-northeast-1.compute.internal"
				],
				"PrivateDNSName": "ip-10-0-130-95.ap-northeast-1.compute.internal",
				"SubnetGatewayIpv4Address": "10.0.130.1/24"
			  }
			],
			"ContainerARN": "arn:aws:ecs:ap-northeast-1:123456789012:container/example/7de2c0ca988f4162be5606783ffd0f6c/72d0588f-609e-4824-b565-b00d034a7f22",
			"LogOptions": {
			  "awslogs-group": "/foobar/example",
			  "awslogs-region": "ap-northeast-1",
			  "awslogs-stream": "development/example/7de2c0ca988f4162be5606783ffd0f6c"
			},
			"LogDriver": "awslogs"
		  }`)
		if err != nil {
			panic(err)
		}
	}))
	defer ts.Close()

	os.Setenv("ECS_CONTAINER_METADATA_URI_V4", ts.URL)
	defer os.Unsetenv("ECS_CONTAINER_METADATA_URI_V4")

	c := newMetadataFetcher()
	if c == nil {
		t.Fatalf("failed to initialize fetcher")
	}
	meta, err := c.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	const containerARN = "arn:aws:ecs:ap-northeast-1:123456789012:container/example/7de2c0ca988f4162be5606783ffd0f6c/72d0588f-609e-4824-b565-b00d034a7f22"
	if meta.ContainerARN != containerARN {
		t.Errorf("unexpected container arn: want %q, got %q", containerARN, meta.ContainerARN)
	}

	const logGroup = "/foobar/example"
	if meta.LogOptions.AWSLogsGroup != logGroup {
		t.Errorf("unexpected log group: want %q, got %q", logGroup, meta.LogOptions.AWSLogsGroup)
	}
	const logRegion = "ap-northeast-1"
	if meta.LogOptions.AWSLogsRegion != logRegion {
		t.Errorf("unexpected log region: want %q, got %q", logRegion, meta.LogOptions.AWSLogsRegion)
	}

	if want, got := "123456789012", meta.AccountID(); got != want {
		t.Errorf("unexpected account id: want %s, got %s", want, got)
	}

	logs := meta.LogReferences()
	if len(logs) != 1 {
		t.Errorf("unexpected logs count: want 1, got %d", len(logs))
	}
	if logs[0].LogGroup != logGroup {
		t.Errorf("unexpected log group: want %q, got %q", logGroup, logs[0].LogGroup)
	}

	logArn := "arn:aws:logs:ap-northeast-1:123456789012:log-group:/foobar/example"
	if logs[0].ARN != logArn {
		t.Errorf("unexpected log group: want %q, got %q", logArn, logs[0].ARN)
	}
}
