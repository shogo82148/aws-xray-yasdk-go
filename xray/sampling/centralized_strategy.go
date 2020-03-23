package sampling

import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	xraySvc "github.com/aws/aws-sdk-go/service/xray"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/xraylog"
)

const defaultInterval = int64(10)
const manifestTTL = 3600 // Seconds

type xrayAPI interface {
	GetSamplingRulesPagesWithContext(aws.Context, *xraySvc.GetSamplingRulesInput, func(*xraySvc.GetSamplingRulesOutput, bool) bool, ...request.Option) error
	GetSamplingTargetsWithContext(aws.Context, *xraySvc.GetSamplingTargetsInput, ...request.Option) (*xraySvc.GetSamplingTargetsOutput, error)
}

// CentralizedStrategy is an implementation of SamplingStrategy.
type CentralizedStrategy struct {
	// Sampling strategy used if centralized manifest is expired
	fallback *LocalizedStrategy

	// AWS X-Ray client
	xray xrayAPI

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

	x, err := newXRaySvc(addr)
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
		xray:         x,
		clientID:     fmt.Sprintf("%x", r),
		pollerCtx:    pollerCtx,
		pollerCancel: pollerCancel,
		manifest: &centralizedManifest{
			Rules:  []*centralizedRule{},
			Quotas: make(map[string]*centralizedQuota),
		},
	}, nil
}

// newXRaySvc returns a new AWS X-Ray client that connects to addr.
// The requests are unsigned and it is expected that the XRay daemon signs and forwards the requests.
func newXRaySvc(addr string) (*xraySvc.XRay, error) {
	url := "http://" + addr
	// Endpoint resolver for proxying requests through the daemon
	f := func(service, region string, optFns ...func(*endpoints.Options)) (endpoints.ResolvedEndpoint, error) {
		return endpoints.ResolvedEndpoint{
			URL: url,
		}, nil
	}

	// Dummy session for unsigned requests
	sess, err := session.NewSession(&aws.Config{
		Region:           aws.String("us-west-1"),
		Credentials:      credentials.NewStaticCredentials("", "", ""),
		EndpointResolver: endpoints.ResolverFunc(f),
	})
	if err != nil {
		return nil, err
	}
	x := xraySvc.New(sess)

	// Remove Signer and replace with No-Op handler
	x.Handlers.Sign.Clear()
	x.Handlers.Sign.PushBack(func(*request.Request) {
		// do nothing
	})

	return x, nil
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

// start should be called by `s.startOnce.Do(s.start)``
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
	err := s.xray.GetSamplingRulesPagesWithContext(ctx, &xraySvc.GetSamplingRulesInput{}, func(out *xraySvc.GetSamplingRulesOutput, lastPage bool) bool {
		for _, record := range out.SamplingRuleRecords {
			r := record.SamplingRule
			name := aws.StringValue(r.RuleName)
			quota, ok := manifest.Quotas[name]
			if !ok {
				// we don't have enough sampling statistics,
				// so borrow the reservoir quota.
				quota = &centralizedQuota{
					fixedRate: aws.Float64Value(r.FixedRate),
				}
			}
			rule := &centralizedRule{
				quota:       quota,
				ruleName:    name,
				priority:    aws.Int64Value(r.Priority),
				host:        aws.StringValue(r.Host),
				httpMethod:  aws.StringValue(r.HTTPMethod),
				serviceName: aws.StringValue(r.ServiceName),
				serviceType: aws.StringValue(r.ServiceType),
			}
			rules = append(rules, rule)
			quotas[name] = quota
			xraylog.Debugf(
				ctx,
				"Refresh Sampling Rule: Name: %s, Priority: %d, FixedRate: %f, Host: %s, Method: %s, ServiceName: %s, ServiceType: %s",
				name, aws.Int64Value(r.Priority), aws.Float64Value(r.FixedRate),
				aws.StringValue(r.Host), aws.StringValue(r.HTTPMethod), aws.StringValue(r.ServiceName), aws.StringValue(r.ServiceType),
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
	stats := make([]*xraySvc.SamplingStatisticsDocument, 0, len(manifest.Rules))
	for _, r := range manifest.Rules {
		stat := r.quota.Stats()
		stats = append(stats, &xraySvc.SamplingStatisticsDocument{
			ClientID:     &s.clientID,
			RuleName:     &r.ruleName,
			RequestCount: &stat.requests,
			SampledCount: &stat.sampled,
			BorrowCount:  &stat.borrowed,
			Timestamp:    &now,
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
		resp, err := s.xray.GetSamplingTargetsWithContext(ctx, &xraySvc.GetSamplingTargetsInput{
			SamplingStatisticsDocuments: stats[:l],
		})
		stats = stats[l:]
		if err != nil {
			xraylog.Errorf(ctx, "failed to refresh sampling targets: %v", err)
			continue
		}
		for _, doc := range resp.SamplingTargetDocuments {
			if quota, ok := manifest.Quotas[aws.StringValue(doc.RuleName)]; ok {
				quota.Update(doc)
				xraylog.Debugf(
					ctx,
					"Refresh Quota: Name: %s, Quota: %d, TTL: %s, Interval: %d",
					aws.StringValue(doc.RuleName), aws.Int64Value(doc.ReservoirQuota), doc.ReservoirQuotaTTL, aws.Int64Value(doc.Interval),
				)
			} else {
				// new rule may be added? try to refresh.
				needRefresh = true
			}
		}
		// check the rules are updated.
		needRefresh = needRefresh || aws.TimeValue(resp.LastRuleModification).After(manifest.RefreshedAt)
	}

	xraylog.Debug(ctx, "sampling targets are refreshed.")

	// TODO update the interval.

	if needRefresh {
		xraylog.Debug(ctx, "chaning sampling rules is detected. refresh them.")
		go s.refreshRule()
	}
}
