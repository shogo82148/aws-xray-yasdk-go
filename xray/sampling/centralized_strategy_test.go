package sampling

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

var _ Strategy = (*CentralizedStrategy)(nil)

func TestCentralizedStrategy_refreshRule(t *testing.T) {
	chRules := make(chan *getSamplingRulesOutput, 1) // rules that return from X-Ray daemon
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			t.Errorf("unexpected method: want %s, got %s", http.MethodPost, req.Method)
		}
		if req.URL.Path != "/GetSamplingRules" {
			t.Errorf("unexpected path: want %s, got %s", "/GetSamplingRules", req.URL.Path)
		}
		if req.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected Content-Type: want %s, got %s", "application/json", req.Header.Get("Content-Type"))
		}

		dec := json.NewDecoder(req.Body)
		var input *getSamplingRulesInput
		if err := dec.Decode(&input); err != nil {
			t.Errorf("decode error: %v", err)
			http.Error(w, "decode error", http.StatusInternalServerError)
			return
		}
		if input.NextToken != "" {
			t.Errorf("unexpected NextToken: want %q, got %q", "", input.NextToken)
		}

		rule := <-chRules
		enc := json.NewEncoder(w)
		if err := enc.Encode(rule); err != nil {
			t.Errorf("encode error: %v", err)
		}
	}))
	defer ts.Close()

	s, err := NewCentralizedStrategy(strings.TrimPrefix(ts.URL, "http://"), nil)
	if err != nil {
		t.Fatal(err)
	}

	chRules <- &getSamplingRulesOutput{
		SamplingRuleRecords: []*samplingRuleRecord{
			{
				SamplingRule: samplingRule{
					Version:       1,
					RuleName:      "Test",
					FixedRate:     0.5,
					HTTPMethod:    "GET",
					Host:          "example.com",
					ReservoirSize: 10,
					RuleARN:       "*",
					ServiceName:   "FooBar",
					ServiceType:   "AWS::EC2::Instance",
				},
			},
		},
	}
	s.refreshRule()

	if len(s.manifest.Rules) != 1 {
		t.Errorf("want %d, got %d", 1, len(s.manifest.Rules))
	}
	r := s.manifest.Rules[0]
	if r.ruleName != "Test" {
		t.Errorf("unexpected rule name: want %q, got %q", "Test", r.ruleName)
	}
	if r.quota.fixedRate != 0.5 {
		t.Errorf("unexpected fix rate: want %f, got %f", 0.5, r.quota.fixedRate)
	}
	if r.quota.quota != 0 {
		t.Errorf("unexpected fix quota: want %d, got %d", 0, r.quota.quota)
	}
	if r.httpMethod != "GET" {
		t.Errorf("unexpected http method: want %q, got %q", "GET", r.httpMethod)
	}
	if r.host != "example.com" {
		t.Errorf("unexpected host name: want %q, got %q", "example.com", r.host)
	}
	if r.serviceName != "FooBar" {
		t.Errorf("unexpected service name: want %q, got %q", "FooBar", r.serviceName)
	}
	if r.serviceType != "AWS::EC2::Instance" {
		t.Errorf("unexpected service type: want %q, got %q", "AWS::EC2::Instance", r.serviceType)
	}
	quota := s.manifest.Quotas["Test"]
	if quota == nil {
		t.Error("want not nil, got nil")
	}

	chRules <- &getSamplingRulesOutput{
		SamplingRuleRecords: []*samplingRuleRecord{
			{
				SamplingRule: samplingRule{
					Version:       1,
					RuleName:      "Test",
					FixedRate:     1.0,
					HTTPMethod:    "*",
					Host:          "*",
					ReservoirSize: 10,
					RuleARN:       "*",
					ServiceName:   "*",
					ServiceType:   "*",
				},
			},
		},
	}
	s.refreshRule()

	if len(s.manifest.Rules) != 1 {
		t.Errorf("want %d, got %d", 1, len(s.manifest.Rules))
	}
	r = s.manifest.Rules[0]
	if r.ruleName != "Test" {
		t.Errorf("unexpected rule name: want %q, got %q", "Test", r.ruleName)
	}
	if s.manifest.Quotas["Test"] != quota {
		t.Error("want quota not to be changed, but changed")
	}
}

func TestCentralizedStrategy_refreshQuota(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			t.Errorf("unexpected method: want %s, got %s", http.MethodPost, req.Method)
		}
		if req.URL.Path != "/SamplingTargets" {
			t.Errorf("unexpected path: want %s, got %s", "/SamplingTargets", req.URL.Path)
		}
		if req.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected Content-Type: want %s, got %s", "application/json", req.Header.Get("Content-Type"))
		}

		dec := json.NewDecoder(req.Body)
		var input *getSamplingTargetsInput
		if err := dec.Decode(&input); err != nil {
			t.Errorf("decode error: %v", err)
			http.Error(w, "decode error", http.StatusInternalServerError)
			return
		}
		input.SamplingStatisticsDocuments[0].Timestamp = "" // ignore timestamp
		want := &getSamplingTargetsInput{
			SamplingStatisticsDocuments: []*samplingStatisticsDocument{
				{
					ClientID:     "client-id-for-test",
					RuleName:     "FooBar",
					BorrowCount:  10,
					SampledCount: 20,
					RequestCount: 30,
				},
			},
		}
		if diff := cmp.Diff(want, input); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}

		enc := json.NewEncoder(w)
		output := &getSamplingTargetsOutput{
			SamplingTargetDocuments: []*samplingTargetDocument{
				{
					RuleName:          "FooBar",
					ReservoirQuota:    13,
					FixedRate:         0.5,
					ReservoirQuotaTTL: "2001-09-09T01:46:40Z", // 1000000000 in unix epoch
					Interval:          15,
				},
			},
		}
		if err := enc.Encode(output); err != nil {
			t.Errorf("encode error: %v", err)
		}
	}))
	defer ts.Close()

	s, err := NewCentralizedStrategy(strings.TrimPrefix(ts.URL, "http://"), nil)
	if err != nil {
		t.Fatal(err)
	}
	s.clientID = "client-id-for-test"
	quota := &centralizedQuota{
		requests: 30,
		borrowed: 10,
		sampled:  20,
	}
	s.manifest = &centralizedManifest{
		Rules: []*centralizedRule{
			{
				quota:    quota,
				ruleName: "FooBar",
			},
		},
		Quotas: map[string]*centralizedQuota{
			"FooBar": quota,
		},
		RefreshedAt: time.Now(),
	}

	s.refreshQuota()

	if quota.fixedRate != 0.5 {
		t.Errorf("unexpected fixed rate: want %f, got %f", 0.5, quota.fixedRate)
	}
	if quota.quota != 13 {
		t.Errorf("unexpected quota: want %d, got %d", 13, quota.quota)
	}
	if quota.ttl.Unix() != 1000000000 {
		t.Errorf("unexpected ttl: want %d, got %d", 1000000000, quota.ttl.Unix())
	}
}

func TestIsDirectIPAccess(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"example.com", false},
		{"example.com:80", false},
		{"192.0.2.1", true},
		{"192.0.2.1:80", true},
		{"198.51.100.1", true},
		{"198.51.100.1:80", true},
		{"2001:db8::1", true},
		{"[2001:db8::1]:80", true},
	}

	for _, tt := range tests {
		req := &Request{
			Host: tt.input,
		}
		if got := isDirectIPAccess(req); got != tt.want {
			t.Errorf("unexpected result: want %t, got %t", tt.want, got)
		}
	}
}
