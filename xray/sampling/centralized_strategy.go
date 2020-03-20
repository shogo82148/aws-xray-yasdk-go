package sampling

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	xraySvc "github.com/aws/aws-sdk-go/service/xray"
)

const defaultRule = "Default"
const defaultInterval = int64(10)

const manifestTTL = 3600 // Seconds

// CentralizedStrategy is an implementation of SamplingStrategy.
type CentralizedStrategy struct {
	// Sampling strategy used if centralized manifest is expired
	fallback *LocalizedStrategy

	// AWS X-Ray client
	xray *xraySvc.XRay

	// Unique ID used by XRay service to identify this client
	clientID string

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

	return &CentralizedStrategy{
		fallback: local,
		xray:     x,
		clientID: fmt.Sprintf("%x", r),
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

}

// ShouldTrace implements Strategy.
func (s *CentralizedStrategy) ShouldTrace(req *Request) *Decision {
	// TODO: @shogo82148 implement me!
	ctx := context.Background()
	s.xray.GetSamplingRulesWithContext(ctx, &xraySvc.GetSamplingRulesInput{})
	s.xray.GetSamplingTargetsWithContext(ctx, &xraySvc.GetSamplingTargetsInput{
		SamplingStatisticsDocuments: nil, // TODO: fill me
	})
	return s.fallback.ShouldTrace(req)
}
