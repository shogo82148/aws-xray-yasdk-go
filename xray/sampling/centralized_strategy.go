package sampling

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/shogo82148/aws-xray-yasdk-go/xray/xraylog"
)

const defaultInterval = int64(10)
const manifestTTL = 3600 // Seconds

var client = &http.Client{
	Transport: &http.Transport{
		Proxy: nil, // ignore proxy configure from the environment values
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          5,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	},
}

// https://docs.aws.amazon.com/xray/latest/api/API_GetSamplingRules.html#API_GetSamplingRules_RequestBody
type getSamplingRulesInput struct {
	NextToken string `json:"NextToken,omitempty"`
}

// https://docs.aws.amazon.com/xray/latest/api/API_GetSamplingRules.html#API_GetSamplingRules_ResponseSyntax
type getSamplingRulesOutput struct {
	NextToken           string `json:"NextToken,omitempty"`
	SamplingRuleRecords []*samplingRuleRecord
}

type samplingRuleRecord struct {
	CreatedAt    float64      `json:"CreatedAt"`
	ModifiedAt   float64      `json:"ModifiedAt"`
	SamplingRule samplingRule `json:"SamplingRule"`
}

type samplingRule struct {
	// Matches attributes derived from the request.
	Attributes map[string]string `json:"Attributes"`

	// The percentage of matching requests to instrument, after the reservoir is
	// exhausted.
	FixedRate float64 `json:"FixedRate"`

	// Matches the HTTP method of a request.
	HTTPMethod string `json:"HTTPMethod"`

	// Matches the hostname from a request URL.
	Host string `json:"Host"`

	// The priority of the sampling rule.
	Priority int64 `json:"Priority"`

	// A fixed number of matching requests to instrument per second, prior to applying
	// the fixed rate. The reservoir is not used directly by services, but applies
	// to all services using the rule collectively.
	ReservoirSize int64 `json:"ReservoirSize"`

	// Matches the ARN of the AWS resource on which the service runs.
	ResourceARN string `json:"ResourceARN"`

	// The ARN of the sampling rule. Specify a rule by either name or ARN, but not
	// both.
	RuleARN string `json:"RuleARN"`

	// The name of the sampling rule. Specify a rule by either name or ARN, but
	// not both.
	RuleName string `json:"RuleName"`

	// Matches the name that the service uses to identify itself in segments.
	ServiceName string `json:"ServiceName"`

	// Matches the origin that the service uses to identify its type in segments.
	ServiceType string `json:"ServiceType"`

	// Matches the path from a request URL.
	URLPath string `json:"URLPath"`

	// The version of the sampling rule format (1).
	Version int64 `json:"Version"`
}

// https://docs.aws.amazon.com/xray/latest/api/API_GetSamplingTargets.html#API_GetSamplingTargets_RequestBody
type getSamplingTargetsInput struct {
	SamplingStatisticsDocuments []*samplingStatisticsDocument `json:"SamplingStatisticsDocuments"`
}

type samplingStatisticsDocument struct {
	// The number of requests recorded with borrowed reservoir quota.
	BorrowCount int64 `json:"BorrowCount"`

	// A unique identifier for the service in hexadecimal.
	ClientID string `json:"ClientID"`

	// The number of requests that matched the rule.
	RequestCount int64 `json:"RequestCount"`

	// The name of the sampling rule.
	RuleName string `json:"RuleName"`

	// The number of requests recorded.
	SampledCount int64 `json:"SampledCount"`

	// The current time, in ISO-8601 format (YYYY-MM-DDThh:mm:ss).
	Timestamp string `json:"Timestamp"`
}

// https://docs.aws.amazon.com/xray/latest/api/API_GetSamplingTargets.html#API_GetSamplingTargets_ResponseSyntax
type getSamplingTargetsOutput struct {
	// The last time a user changed the sampling rule configuration. If the sampling
	// rule configuration changed since the service last retrieved it, the service
	// should call GetSamplingRules to get the latest version.
	LastRuleModification int64 `json:"LastRuleModification"`

	// Updated rules that the service should use to sample requests.
	SamplingTargetDocuments []*samplingTargetDocument `json:"SamplingTargetDocuments"`

	// Information about SamplingStatisticsDocument that X-Ray could not process.
	UnprocessedStatistics []*unprocessedStatistics `json:"UnprocessedStatistics"`
}

type samplingTargetDocument struct {
	// The percentage of matching requests to instrument, after the reservoir is
	// exhausted.
	FixedRate float64 `json:"FixedRate"`

	// The number of seconds for the service to wait before getting sampling targets
	// again.
	Interval int64 `json:"Interval"`

	// The number of requests per second that X-Ray allocated this service.
	ReservoirQuota int64 `json:"ReservoirQuota"`

	// When the reservoir quota expires, in ISO-8601 format (YYYY-MM-DDThh:mm:ss).
	ReservoirQuotaTTL string `json:"ReservoirQuotaTTL"`

	// The name of the sampling rule.
	RuleName string `json:"RuleName"`
}

type unprocessedStatistics struct {
	// The error code.
	ErrorCode string `json:"ErrorCode"`

	// The error message.
	Message string `json:"Message"`

	// The name of the sampling rule.
	RuleName string `json:"RuleName"`
}

// CentralizedStrategy is an implementation of SamplingStrategy.
type CentralizedStrategy struct {
	// Sampling strategy used if centralized manifest is expired
	fallback *LocalizedStrategy

	// Address for X-Ray daemon
	addr string

	// Unique ID used by XRay service to identify this client
	clientID string

	// control poller
	pollerCtx    context.Context
	pollerCancel context.CancelFunc
	startOnce    sync.Once
	muRefresh    sync.Mutex

	mu       sync.RWMutex
	manifest *centralizedManifest
}

// NewCentralizedStrategy returns new centralized sampling strategy with a fallback on
// the local rule. If local rule is nil, the DefaultSamplingRule is used.
func NewCentralizedStrategy(addr string, manifest *Manifest) (*CentralizedStrategy, error) {
	local, err := NewLocalizedStrategy(manifest)
	if err != nil {
		return nil, err
	}

	// Generate clientID
	var r [12]byte
	if _, err := crand.Read(r[:]); err != nil {
		return nil, err
	}

	pollerCtx, pollerCancel := context.WithCancel(context.Background())

	return &CentralizedStrategy{
		fallback:     local,
		addr:         addr,
		clientID:     fmt.Sprintf("%x", r),
		pollerCtx:    pollerCtx,
		pollerCancel: pollerCancel,
		manifest: &centralizedManifest{
			Rules:  []*centralizedRule{},
			Quotas: make(map[string]*centralizedQuota),
		},
	}, nil
}

func (s *CentralizedStrategy) getSamplingRulesPages(ctx context.Context, input *getSamplingRulesInput, callback func(*getSamplingRulesOutput, bool) bool) error {
	token := input.NextToken
	for {
		out, err := s.getSamplingRules(ctx, &getSamplingRulesInput{
			NextToken: token,
		})
		if err != nil {
			return err
		}
		lastPage := out.NextToken == ""
		if !callback(out, lastPage) || lastPage {
			break
		}
		token = out.NextToken
	}
	return nil
}

// Retrieves all sampling rules.
//
// https://docs.aws.amazon.com/xray/latest/api/API_GetSamplingRules.html
func (s *CentralizedStrategy) getSamplingRules(ctx context.Context, input *getSamplingRulesInput) (*getSamplingRulesOutput, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, "http://"+s.addr+"/GetSamplingRules", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	dec := json.NewDecoder(resp.Body)
	var output getSamplingRulesOutput
	if err := dec.Decode(&output); err != nil {
		return nil, err
	}

	return &output, nil
}

// Requests a sampling quota for rules that the service is using to sample requests.
//
// https://docs.aws.amazon.com/xray/latest/api/API_GetSamplingTargets.html
func (s *CentralizedStrategy) getSamplingTargets(ctx context.Context, input *getSamplingTargetsInput) (*getSamplingTargetsOutput, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, "http://"+s.addr+"/SamplingTargets", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	dec := json.NewDecoder(resp.Body)
	var output getSamplingTargetsOutput
	if err := dec.Decode(&output); err != nil {
		return nil, err
	}

	return &output, nil
}

// Close stops polling.
func (s *CentralizedStrategy) Close() {
	s.pollerCancel()
}

// ShouldTrace implements Strategy.
func (s *CentralizedStrategy) ShouldTrace(req *Request) *Decision {
	s.startOnce.Do(s.start)
	manifest := s.getManifest()
	if manifest == nil {
		return s.fallback.ShouldTrace(req)
	}

	for _, r := range manifest.Rules {
		if r.Match(req) {
			xraylog.Debugf(context.Background(), "ShouldTrace Match: rule %s", r.ruleName)
			return r.Sample()
		}
	}

	// It should not reach here, because the Default Rule matches any requests.
	// The manifest is wrong, so fallback to local strategy.
	return s.fallback.ShouldTrace(req)
}

func (s *CentralizedStrategy) getManifest() *centralizedManifest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.manifest
}

func (s *CentralizedStrategy) setManifest(manifest *centralizedManifest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.manifest = manifest
}

// start should be called by `s.startOnce.Do(s.start)“
func (s *CentralizedStrategy) start() {
	go s.rulePoller()
	go s.quotaPoller()
}

func (s *CentralizedStrategy) rulePoller() {
	var seed int64
	if err := binary.Read(crand.Reader, binary.BigEndian, &seed); err != nil {
		// fallback to timestamp
		seed = time.Now().UnixNano()
	}
	rnd := rand.New(rand.NewSource(seed))
	interval := 300 * time.Second
	jitter := int64(time.Second)

	for {
		s.refreshRule()

		timer := time.NewTimer(interval + time.Duration(rnd.Int63n(jitter)))
		select {
		case <-s.pollerCtx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (s *CentralizedStrategy) quotaPoller() {
	var seed int64
	if err := binary.Read(crand.Reader, binary.BigEndian, &seed); err != nil {
		// fallback to timestamp
		seed = time.Now().UnixNano()
	}
	rnd := rand.New(rand.NewSource(seed))
	interval := 10 * time.Second
	jitter := int64(100 * time.Millisecond)

	for {
		s.refreshQuota()

		timer := time.NewTimer(interval + time.Duration(rnd.Int63n(jitter)))
		select {
		case <-s.pollerCtx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (s *CentralizedStrategy) refreshRule() {
	ctx, cancel := context.WithTimeout(s.pollerCtx, time.Minute)
	defer cancel()
	s.muRefresh.Lock()
	defer s.muRefresh.Unlock()
	defer func() {
		// avoid propagating panics to the application code.
		if e := recover(); e != nil {
			xraylog.Errorf(ctx, "panic: %v", e)
		}
	}()

	xraylog.Debug(ctx, "start refreshing sampling rules")
	manifest := s.getManifest()
	rules := make([]*centralizedRule, 0, len(manifest.Rules))
	quotas := make(map[string]*centralizedQuota, len(manifest.Rules))
	err := s.getSamplingRulesPages(ctx, &getSamplingRulesInput{}, func(out *getSamplingRulesOutput, lastPage bool) bool {
		for _, record := range out.SamplingRuleRecords {
			r := record.SamplingRule
			name := r.RuleName
			quota, ok := manifest.Quotas[name]
			if !ok {
				// we don't have enough sampling statistics,
				// so borrow the reservoir quota.
				quota = &centralizedQuota{
					fixedRate: r.FixedRate,
				}
			}
			rule := &centralizedRule{
				quota:       quota,
				ruleName:    name,
				priority:    r.Priority,
				host:        r.Host,
				urlPath:     r.URLPath,
				httpMethod:  r.HTTPMethod,
				serviceName: r.ServiceName,
				serviceType: r.ServiceType,
			}
			rules = append(rules, rule)
			quotas[name] = quota
			xraylog.Debugf(
				ctx,
				"Refresh Sampling Rule: Priority: %d, ServiceName: %s, ServiceType: %s, Name: %s, Host: %s, URL: %s, Method: %s, Quota: %d, FixedRate: %f",
				r.Priority, r.ServiceName, r.ServiceType,
				name, r.Host, r.HTTPMethod, r.URLPath, quota.quota, r.FixedRate,
			)
		}
		return true
	})
	if err != nil {
		xraylog.Errorf(ctx, "xray/sampling: failed to get sampling rules: %v", err)
		return
	}
	sort.Stable(centralizedRuleSlice(rules))

	s.setManifest(&centralizedManifest{
		Rules:       rules,
		Quotas:      quotas,
		RefreshedAt: time.Now(),
	})
	xraylog.Debug(ctx, "sampling rules are refreshed.")
}

func (s *CentralizedStrategy) refreshQuota() {
	// maximum number of targets of GetSamplingTargets API
	const maxTargets = 25

	ctx, cancel := context.WithTimeout(s.pollerCtx, time.Minute)
	defer cancel()
	s.muRefresh.Lock()
	defer s.muRefresh.Unlock()
	defer func() {
		// avoid propagating panics to the application code.
		if e := recover(); e != nil {
			xraylog.Errorf(ctx, "panic: %v", e)
		}
	}()

	manifest := s.getManifest()
	now := time.Now()
	stats := make([]*samplingStatisticsDocument, 0, len(manifest.Rules))
	for _, r := range manifest.Rules {
		stat := r.quota.Stats()
		stats = append(stats, &samplingStatisticsDocument{
			ClientID:     s.clientID,
			RuleName:     r.ruleName,
			RequestCount: stat.requests,
			SampledCount: stat.sampled,
			BorrowCount:  stat.borrowed,
			Timestamp:    now.Format(time.RFC3339),
		})
		xraylog.Debugf(
			ctx,
			"Sampling Statistics: Name: %s, Requests: %d, Borrowed: %d, Sampled: %d", r.ruleName, stat.requests, stat.borrowed, stat.sampled,
		)
	}

	var needRefresh bool
	for len(stats) > 0 {
		l := len(stats)
		if l > maxTargets {
			l = maxTargets
		}
		resp, err := s.getSamplingTargets(ctx, &getSamplingTargetsInput{
			SamplingStatisticsDocuments: stats[:l],
		})
		stats = stats[l:]
		if err != nil {
			xraylog.Errorf(ctx, "failed to refresh sampling targets: %v", err)
			continue
		}
		for _, doc := range resp.SamplingTargetDocuments {
			if quota, ok := manifest.Quotas[doc.RuleName]; ok {
				if err := quota.update(doc); err != nil {
					xraylog.Errorf(
						ctx,
						"Failed to Refresh Quota: Name: %s, Quota: %d, TTL: %s, Interval: %d, Error: %v",
						doc.RuleName, doc.ReservoirQuota, doc.ReservoirQuotaTTL, doc.Interval, err,
					)
					continue
				}
				xraylog.Debugf(
					ctx,
					"Refresh Quota: Name: %s, Quota: %d, TTL: %s, Interval: %d",
					doc.RuleName, doc.ReservoirQuota, doc.ReservoirQuotaTTL, doc.Interval,
				)
			} else {
				// new rule may be added? try to refresh.
				needRefresh = true
			}
		}
		// check the rules are updated.
		lastModification := time.Unix(resp.LastRuleModification, 0)
		needRefresh = needRefresh || manifest.RefreshedAt.IsZero() || lastModification.After(manifest.RefreshedAt)
	}

	xraylog.Debug(ctx, "sampling targets are refreshed.")

	// TODO update the interval.

	if needRefresh {
		xraylog.Debug(ctx, "chaning sampling rules is detected. refresh them.")
		go s.refreshRule()
	}
}
