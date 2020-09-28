package ec2

import (
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

func TestGetInstanceIdentityDocument_IMDSv1(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/latest/api/token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	mux.HandleFunc("/latest/dynamic/instance-identity/document", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected http method: want %s, got %s", http.MethodGet, r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err := io.WriteString(w, `{
			"devpayProductCodes" : null,
			"marketplaceProductCodes" : null,
			"accountId" : "445285296882",
			"availabilityZone" : "ap-northeast-1a",
			"kernelId" : null,
			"ramdiskId" : null,
			"pendingTime" : "2019-04-30T06:52:00Z",
			"architecture" : "x86_64",
			"privateIp" : "10.0.0.207",
			"version" : "2017-09-30",
			"region" : "ap-northeast-1",
			"imageId" : "ami-0f9ae750e8274075b",
			"billingProducts" : null,
			"instanceId" : "i-009df055e1f06d17f",
			"instanceType" : "t3.micro"
		  }`)
		if err != nil {
			t.Error(err)
		}
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	c := &client{
		base: ts.URL,
	}
	got, err := c.getInstanceIdentityDocument(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := &ec2InstanceIdentityDocument{
		AvailabilityZone: "ap-northeast-1a",
		PrivateIP:        "10.0.0.207",
		Version:          "2017-09-30",
		Region:           "ap-northeast-1",
		InstanceID:       "i-009df055e1f06d17f",
		InstanceType:     "t3.micro",
		AccountID:        "445285296882",
		PendingTime:      time.Date(2019, time.April, 30, 6, 52, 0, 0, time.UTC),
		ImageID:          "ami-0f9ae750e8274075b",
		Architecture:     "x86_64",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("-want/+got:\n%s", diff)
	}
}

func TestGetInstanceIdentityDocument_IMDSv2(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/latest/api/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("unexpected http method: want %s, got %s", http.MethodPut, r.Method)
		}
		if r.Header.Get("x-aws-ec2-metadata-token-ttl-seconds") != "10" {
			t.Errorf("want %s, got %s", "10", r.Header.Get("x-aws-ec2-metadata-token-ttl-seconds"))
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		if _, err := io.WriteString(w, "token-for-IMDSv2"); err != nil {
			t.Error(err)
		}
	})
	mux.HandleFunc("/latest/dynamic/instance-identity/document", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected http method: want %s, got %s", http.MethodGet, r.Method)
		}
		if r.Header.Get("x-aws-ec2-metadata-token") != "token-for-IMDSv2" {
			t.Errorf("want %s, got %s", "token-for-IMDSv2", r.Header.Get("x-aws-ec2-metadata-token"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err := io.WriteString(w, `{
			"devpayProductCodes" : null,
			"marketplaceProductCodes" : null,
			"accountId" : "445285296882",
			"availabilityZone" : "ap-northeast-1a",
			"kernelId" : null,
			"ramdiskId" : null,
			"pendingTime" : "2019-04-30T06:52:00Z",
			"architecture" : "x86_64",
			"privateIp" : "10.0.0.207",
			"version" : "2017-09-30",
			"region" : "ap-northeast-1",
			"imageId" : "ami-0f9ae750e8274075b",
			"billingProducts" : null,
			"instanceId" : "i-009df055e1f06d17f",
			"instanceType" : "t3.micro"
		  }`)
		if err != nil {
			t.Error(err)
		}
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	c := &client{
		base: ts.URL,
	}
	got, err := c.getInstanceIdentityDocument(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := &ec2InstanceIdentityDocument{
		AvailabilityZone: "ap-northeast-1a",
		PrivateIP:        "10.0.0.207",
		Version:          "2017-09-30",
		Region:           "ap-northeast-1",
		InstanceID:       "i-009df055e1f06d17f",
		InstanceType:     "t3.micro",
		AccountID:        "445285296882",
		PendingTime:      time.Date(2019, time.April, 30, 6, 52, 0, 0, time.UTC),
		ImageID:          "ami-0f9ae750e8274075b",
		Architecture:     "x86_64",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("-want/+got:\n%s", diff)
	}
}

func TestParseAgentConfig(t *testing.T) {
	// prepare configure file for test.
	tmp, err := ioutil.TempFile("", "*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp.Name())
	_, err = io.WriteString(tmp, `{`+
		`"log_group_name": "group1",`+ // top level object
		`"foo": {`+
		`"log_group_name": "group2"`+ // object in object
		`},`+
		`"bar": [`+
		`"dummy",`+
		`"dummy",`+
		`{"log_group_name": "group3" },`+ // object in array
		`{"log_group_name": 123456 }`+ // the value is a number, it should be ignored.
		`]}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatal(err)
	}

	got := parseAgentConfig(context.Background(), tmp.Name())
	want := []*schema.LogReference{
		{LogGroup: "group1"},
		{LogGroup: "group2"},
		{LogGroup: "group3"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("-want/+got:\n%s", diff)
	}
}

func TestGetInstanceIdentityDocument_IMDSEndpointFromEnv(t *testing.T) {
	var called int32
	mux := http.NewServeMux()
	mux.HandleFunc("/latest/api/token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	mux.HandleFunc("/latest/dynamic/instance-identity/document", func(w http.ResponseWriter, r *http.Request) {
		atomic.StoreInt32(&called, 1)
		if r.Method != http.MethodGet {
			t.Errorf("unexpected http method: want %s, got %s", http.MethodGet, r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err := io.WriteString(w, `{
			"devpayProductCodes" : null,
			"marketplaceProductCodes" : null,
			"accountId" : "445285296882",
			"availabilityZone" : "ap-northeast-1a",
			"kernelId" : null,
			"ramdiskId" : null,
			"pendingTime" : "2019-04-30T06:52:00Z",
			"architecture" : "x86_64",
			"privateIp" : "10.0.0.207",
			"version" : "2017-09-30",
			"region" : "ap-northeast-1",
			"imageId" : "ami-0f9ae750e8274075b",
			"billingProducts" : null,
			"instanceId" : "i-009df055e1f06d17f",
			"instanceType" : "t3.micro"
		  }`)
		if err != nil {
			t.Error(err)
		}
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	os.Setenv("AWS_EC2_METADATA_SERVICE_ENDPOINT", ts.URL)
	defer os.Unsetenv("AWS_EC2_METADATA_SERVICE_ENDPOINT")

	Init()

	if atomic.LoadInt32(&called) != 1 {
		t.Error("IMDSv1 is not called")
	}
}
