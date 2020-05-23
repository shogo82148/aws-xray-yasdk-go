package sampling

import (
	crand "crypto/rand"
	"encoding/binary"
	"math/rand"
	"sync"
	"time"
)

type centralizedManifest struct {
	Rules       []*centralizedRule
	Quotas      map[string]*centralizedQuota
	RefreshedAt time.Time
}

type centralizedRule struct {
	quota *centralizedQuota

	// Rule name identifying this rule
	ruleName string

	// Priority of matching against rule
	priority int64

	// The hostname from the HTTP host header.
	host string

	// The method of the HTTP request.
	httpMethod string

	// The URL path of the request.
	urlPath string

	// The name of the instrumented service, as it appears in the service map.
	serviceName string

	// ServiceType for the sampling rule
	serviceType string
}

func (r *centralizedRule) Match(req *Request) bool {
	if req == nil {
		return true
	}
	return (req.Host == "" || WildcardMatchCaseInsensitive(r.host, req.Host)) &&
		(req.URL == "" || WildcardMatchCaseInsensitive(r.urlPath, req.URL)) &&
		(req.Method == "" || WildcardMatchCaseInsensitive(r.urlPath, req.Method)) &&
		(req.ServiceName == "" || WildcardMatchCaseInsensitive(r.serviceName, req.ServiceName)) &&
		(req.ServiceType == "" || WildcardMatchCaseInsensitive(r.serviceType, req.ServiceType))
}

func (r *centralizedRule) Sample() *Decision {
	return &Decision{
		Rule:   &r.ruleName,
		Sample: r.quota.Sample(),
	}
}

type centralizedQuota struct {
	mu sync.Mutex

	// randFunc returns, as a float64, a pseudo-random number in [0.0,1.0).
	randFunc func() float64

	// returns current time.
	nowFunc func() time.Time

	// The percentage of matching requests to instrument, after the reservoir is
	// exhausted.
	fixedRate float64

	// The number of requests per second that X-Ray allocated this service.
	quota int64

	// When the reservoir quota expires.
	ttl time.Time

	// The number of requests recorded with borrowed reservoir quota.
	borrowed int64

	// The number of requests that matched the rule.
	requests int64

	// The number of requests recorded.
	sampled int64

	// Reservoir consumption for current epoch.
	used int64

	// Unix epoch. Reservoir usage is reset every second.
	currentEpoch int64
}

func (q *centralizedQuota) update(doc *samplingTargetDocument) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.fixedRate = doc.FixedRate
	q.quota = doc.ReservoirQuota
	ttl, err := time.Parse(time.RFC3339Nano, doc.ReservoirQuotaTTL)
	if err != nil {
		return err
	}
	q.ttl = ttl
	return nil
}

// ref. https://docs.aws.amazon.com/xray/latest/devguide/xray-api-sampling.html
func (q *centralizedQuota) Sample() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.requests++

	// reset counters every seconds
	if epoch := q.nowLocked().Unix(); epoch != q.currentEpoch {
		q.currentEpoch = epoch
		q.used = 0
	}

	if q.isExpired() {
		// quota is not available
		// borrow one trace per second from the reservoir
		if q.used < 1 {
			q.borrowed++
			q.sampled++
			q.used++
			return true
		}
	} else {
		// quota is available, and consume it.
		// Take from reservoir, if available
		if q.used < q.quota {
			q.sampled++
			q.used++
			return true
		}
	}

	// fall back to the Bernoulli distribution
	if q.randLocked() < q.fixedRate {
		q.sampled++
		return true
	}
	return false
}

func (q *centralizedQuota) isExpired() bool {
	return q.ttl.IsZero() || !q.ttl.After(q.nowLocked())
}

// returns current time. q.mu should be locked.
func (q *centralizedQuota) nowLocked() time.Time {
	if q.nowFunc != nil {
		return q.nowFunc()
	}
	q.nowFunc = time.Now
	return q.nowFunc()
}

// returns a pseudo-random number in [0.0,1.0). q.mu should be locked.
func (q *centralizedQuota) randLocked() float64 {
	if q.randFunc != nil {
		return q.randFunc()
	}
	// lazy initialize of random generator
	var seed int64
	if err := binary.Read(crand.Reader, binary.BigEndian, &seed); err != nil {
		// fallback to timestamp
		seed = q.nowLocked().UnixNano()
	}
	q.randFunc = rand.New(rand.NewSource(seed)).Float64
	return q.randFunc()
}

type centralizedQuotaStats struct {
	// The number of requests recorded with borrowed reservoir quota.
	borrowed int64

	// The number of requests that matched the rule.
	requests int64

	// The number of requests recorded.
	sampled int64
}

// Stats returns the snapshot of statistics and reset it.
func (q *centralizedQuota) Stats() centralizedQuotaStats {
	q.mu.Lock()
	defer q.mu.Unlock()
	ret := centralizedQuotaStats{
		borrowed: q.borrowed,
		requests: q.requests,
		sampled:  q.sampled,
	}
	q.borrowed = 0
	q.requests = 0
	q.sampled = 0
	return ret
}

type centralizedRuleSlice []*centralizedRule

func (s centralizedRuleSlice) Len() int { return len(s) }
func (s centralizedRuleSlice) Less(i, j int) bool {
	if s[i].priority == s[j].priority {
		return s[i].ruleName < s[j].ruleName
	}
	return s[i].priority < s[j].priority
}
func (s centralizedRuleSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
