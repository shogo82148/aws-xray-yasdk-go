package sampling

import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	xraySvc "github.com/aws/aws-sdk-go/service/xray"
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
		if err := s.refreshRule(); err != nil {
			// TODO: log error
		}

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
		if err := s.refreshQuota(); err != nil {
			// TODO: log error
		}

		timer := time.NewTimer(interval + time.Duration(rnd.Int63n(jitter)))
		select {
		case <-s.pollerCtx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (s *CentralizedStrategy) refreshRule() (err error) {
	defer func() {
		// avoid propagating panics to the application code.
		if e := recover(); e != nil {
			err = fmt.Errorf("panic: %v", e)
		}
	}()
	ctx, cancel := context.WithTimeout(s.pollerCtx, time.Minute)
	defer cancel()
	s.muRefresh.Lock()
	defer s.muRefresh.Unlock()

	manifest := s.getManifest()
	rules := make([]*centralizedRule, 0, len(manifest.Rules))
	quotas := make(map[string]*centralizedQuota, len(manifest.Rules))
	err = s.xray.GetSamplingRulesPagesWithContext(ctx, &xraySvc.GetSamplingRulesInput{}, func(out *xraySvc.GetSamplingRulesOutput, lastPage bool) bool {
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
		}
		return true
	})
	if err != nil {
		return err
	}

	s.setManifest(&centralizedManifest{
		Rules:       rules,
		Quotas:      quotas,
		RefreshedAt: time.Now(),
	})
	return nil
}

func (s *CentralizedStrategy) refreshQuota() (err error) {
	defer func() {
		// avoid propagating panics to the application code.
		if e := recover(); e != nil {
			err = fmt.Errorf("panic: %v", e)
		}
	}()
	ctx, cancel := context.WithTimeout(s.pollerCtx, time.Minute)
	defer cancel()
	s.muRefresh.Lock()
	defer s.muRefresh.Unlock()

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
	}

	resp, err := s.xray.GetSamplingTargetsWithContext(ctx, &xraySvc.GetSamplingTargetsInput{
		SamplingStatisticsDocuments: stats,
	})
	if err != nil {
		return err
	}

	needRefresh := false
	for _, doc := range resp.SamplingTargetDocuments {
		quota, ok := manifest.Quotas[aws.StringValue(doc.RuleName)]
		if !ok {
			// new rule may be added? try to refresh.
			needRefresh = true
		}
		quota.Update(doc)
	}

	// TODO update the interval.

	// check the rules are updated.
	needRefresh = needRefresh || aws.TimeValue(resp.LastRuleModification).After(manifest.RefreshedAt)

	if needRefresh {
		go func() {
			if err := s.refreshRule(); err != nil {
				// TODO: log err
			}
		}()
	}
	return nil
}
