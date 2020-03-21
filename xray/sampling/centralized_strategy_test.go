package sampling

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	xraySvc "github.com/aws/aws-sdk-go/service/xray"
)

var _ Strategy = (*CentralizedStrategy)(nil)

type xrayMock struct {
	getSamplingRulesPagesWithContext func(aws.Context, *xraySvc.GetSamplingRulesInput, func(*xraySvc.GetSamplingRulesOutput, bool) bool, ...request.Option) error
	getSamplingTargetsWithContext    func(aws.Context, *xraySvc.GetSamplingTargetsInput, ...request.Option) (*xraySvc.GetSamplingTargetsOutput, error)
}

func (m *xrayMock) GetSamplingRulesPagesWithContext(ctx aws.Context, in *xraySvc.GetSamplingRulesInput, f func(*xraySvc.GetSamplingRulesOutput, bool) bool, opts ...request.Option) error {
	return m.getSamplingRulesPagesWithContext(ctx, in, f, opts...)
}

func (m *xrayMock) GetSamplingTargetsWithContext(ctx aws.Context, in *xraySvc.GetSamplingTargetsInput, opts ...request.Option) (*xraySvc.GetSamplingTargetsOutput, error) {
	return m.getSamplingTargetsWithContext(ctx, in, opts...)
}

func TestCentralizedStrategy_refreshRule(t *testing.T) {
	s, err := NewCentralizedStrategy("127.0.0.1", nil)
	if err != nil {
		t.Fatal(err)
	}

	s.xray = &xrayMock{
		getSamplingRulesPagesWithContext: func(ctx aws.Context, in *xraySvc.GetSamplingRulesInput, f func(*xraySvc.GetSamplingRulesOutput, bool) bool, opts ...request.Option) error {
			f(&xraySvc.GetSamplingRulesOutput{
				SamplingRuleRecords: []*xraySvc.SamplingRuleRecord{
					{
						SamplingRule: &xraySvc.SamplingRule{
							Version:       aws.Int64(1),
							RuleName:      aws.String("Test"),
							FixedRate:     aws.Float64(0.5),
							HTTPMethod:    aws.String("GET"),
							Host:          aws.String("example.com"),
							ReservoirSize: aws.Int64(10),
							RuleARN:       aws.String("*"),
							ServiceName:   aws.String("FooBar"),
							ServiceType:   aws.String("AWS::EC2::Instance"),
						},
					},
				},
			}, true)
			return nil
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

	s.xray = &xrayMock{
		getSamplingRulesPagesWithContext: func(ctx aws.Context, in *xraySvc.GetSamplingRulesInput, f func(*xraySvc.GetSamplingRulesOutput, bool) bool, opts ...request.Option) error {
			f(&xraySvc.GetSamplingRulesOutput{
				SamplingRuleRecords: []*xraySvc.SamplingRuleRecord{
					{
						SamplingRule: &xraySvc.SamplingRule{
							Version:       aws.Int64(1),
							RuleName:      aws.String("Test"),
							FixedRate:     aws.Float64(1.0),
							HTTPMethod:    aws.String("*"),
							Host:          aws.String("*"),
							ReservoirSize: aws.Int64(10),
							RuleARN:       aws.String("*"),
							ServiceName:   aws.String("*"),
							ServiceType:   aws.String("*"),
						},
					},
				},
			}, true)
			return nil
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
