package sampling

import (
	"math"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	xraySvc "github.com/aws/aws-sdk-go/service/xray"
)

func TestCentralizedQuota_Sample(t *testing.T) {
	var random float64 = 1
	var now int64 = 1000000000 // unix epoch

	quota := &centralizedQuota{
		randFunc: func() float64 { return random },
		nowFunc:  func() time.Time { return time.Unix(now, 0) },
	}
	quota.Update(&xraySvc.SamplingTargetDocument{
		FixedRate:         aws.Float64(0.05),
		ReservoirQuota:    aws.Int64(5),
		ReservoirQuotaTTL: aws.Time(time.Unix(int64(1000000010), 0)),
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
