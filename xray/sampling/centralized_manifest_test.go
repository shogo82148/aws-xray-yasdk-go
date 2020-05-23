package sampling

import (
	"math"
	"sort"
	"testing"
	"time"
)

func TestCentralizedRule_Match(t *testing.T) {
	tc := []struct {
		req  *Request
		rule *centralizedRule
		want bool
	}{
		{
			req: nil,
			rule: &centralizedRule{
				ruleName: "nil Request",
			},
			want: true,
		},
		{
			req: &Request{},
			rule: &centralizedRule{
				ruleName: "zero Request",
			},
			want: true,
		},
		{
			req: &Request{
				Host:        "localhost:8080",
				Method:      "GET",
				URL:         "/",
				ServiceName: "",
				ServiceType: "",
			},
			rule: &centralizedRule{
				ruleName:    "default",
				host:        "*",
				httpMethod:  "*",
				urlPath:     "*",
				serviceName: "*",
				serviceType: "*",
			},
			want: true,
		},
	}
	for _, tt := range tc {
		if tt.rule.Match(tt.req) != tt.want {
			t.Errorf("%s: want %t, got %t", tt.rule.ruleName, tt.want, !tt.want)
		}
	}
}

func TestCentralizedQuota_Sample(t *testing.T) {
	var random float64 = 1
	var now int64 = 1000000000 // unix epoch

	quota := &centralizedQuota{
		randFunc: func() float64 { return random },
		nowFunc:  func() time.Time { return time.Unix(now, 0) },
	}
	quota.update(&samplingTargetDocument{
		FixedRate:         0.05,
		ReservoirQuota:    5,
		ReservoirQuotaTTL: "2001-09-09T01:46:50Z", // 1000000010 in unix epoch
	})

	// first 5 requests consume the quota of the current epoch
	for i := 0; i < 5; i++ {
		if !quota.Sample() {
			t.Errorf("want true, got false")
		}
	}

	// no quota remains, fallback to bernoulli sampling
	random = 0.05
	if quota.Sample() {
		t.Errorf("want false, got true")
	}
	random = math.Nextafter(0.05, 0)
	if !quota.Sample() {
		t.Errorf("want true, got false")
	}

	// next epoch
	now++
	for i := 0; i < 5; i++ {
		if !quota.Sample() {
			t.Errorf("want true, got false")
		}
	}

	// quota is expired, borrow one trace per second from the reservoir
	random = 1
	now = 1000000010
	if !quota.Sample() {
		t.Errorf("want true, got false")
	}
	if quota.Sample() {
		t.Errorf("want false, got true")
	}

	// fallback to bernoulli sampling
	random = 0.05
	if quota.Sample() {
		t.Errorf("want false, got true")
	}
	random = math.Nextafter(0.05, 0)
	if !quota.Sample() {
		t.Errorf("want true, got false")
	}

	stats := quota.Stats()
	if stats.borrowed != 1 {
		t.Errorf("unexpected borrowed: want %d, got%d", 1, stats.borrowed)
	}
	if stats.requests != 16 {
		t.Errorf("unexpected requests: want %d, got%d", 16, stats.requests)
	}
	if stats.sampled != 13 {
		t.Errorf("unexpected borrowed: want %d, got%d", 13, stats.sampled)
	}

	// all stats is cleared
	stats = quota.Stats()
	if stats.borrowed != 0 {
		t.Errorf("unexpected borrowed: want %d, got%d", 0, stats.borrowed)
	}
	if stats.requests != 0 {
		t.Errorf("unexpected requests: want %d, got%d", 0, stats.requests)
	}
	if stats.sampled != 0 {
		t.Errorf("unexpected borrowed: want %d, got%d", 0, stats.sampled)
	}
}

var _ sort.Interface = centralizedRuleSlice(nil)

func TestCentralizedRuleSlice(t *testing.T) {
	rules := []*centralizedRule{
		{
			ruleName: "B",
			priority: 20,
		},
		{
			ruleName: "A",
			priority: 10,
		},
		{
			ruleName: "C",
			priority: 20,
		},
	}
	sort.Stable(centralizedRuleSlice(rules))

	if rules[0].ruleName != "A" {
		t.Errorf("want %q, got %q", "A", rules[0].ruleName)
	}
	if rules[1].ruleName != "B" {
		t.Errorf("want %q, got %q", "B", rules[1].ruleName)
	}
	if rules[2].ruleName != "C" {
		t.Errorf("want %q, got %q", "C", rules[2].ruleName)
	}
}
