package sampling

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Manifest is a list of sampling rules.
type Manifest struct {
	Version int     `json:"version"`
	Default *Rule   `json:"default"`
	Rules   []*Rule `json:"rules"`
}

// Rule is a sampling rule.
type Rule struct {
	// Description
	Description string `json:"description"`

	// The hostname from the HTTP host header.
	Host string `json:"host"`

	// The method of the HTTP request.
	HTTPMethod string `json:"http_method"`

	// The URL path of the request.
	URLPath string `json:"url_path"`

	// The name of the instrumented service, as it appears in the service map.
	ServiceName string `json:"service_name"`

	// FixedTarget
	FixedTarget int64 `json:"fixed_target"`

	// The rate of matching requests to instrument, after the reservoir is exhausted.
	Rate float64 `json:"rate"`
}

// DefaultSamplingRule is default sampling rule, if centralized sampling rule is not available.
var DefaultSamplingRule = &Manifest{
	Version: 2,
	Default: &Rule{
		FixedTarget: 1,
		Rate:        0.05,
	},
	Rules: []*Rule{},
}

// DecodeManifest decodes json-encoded manifest file.
func DecodeManifest(r io.Reader) (*Manifest, error) {
	var manifest Manifest
	dec := json.NewDecoder(r)
	if err := dec.Decode(&manifest); err != nil {
		return nil, err
	}
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return &manifest, nil
}

// Validate checks whether the manifest is valid.
func (m *Manifest) Validate() error {
	if m == nil {
		return errors.New("xray/sampling: sampling rule manifest must not be nil")
	}
	if m.Version != 1 && m.Version != 2 {
		return fmt.Errorf("xray/sampling: sampling rule manifest version %d not supported", m.Version)
	}
	if m.Default == nil {
		return errors.New("xray/sampling: sampling rule manifest must include a default rule")
	}
	if m.Default.URLPath != "" || m.Default.ServiceName != "" || m.Default.HTTPMethod != "" {
		return errors.New("xray/sampling: the default rule must not specify values for url_path, service_name, or http_method")
	}
	if m.Default.FixedTarget < 0 || m.Default.Rate < 0 {
		return errors.New("xray/sampling: the default rule must specify non-negative values for fixed_target and rate")
	}
	switch m.Version {
	case 1:
		for _, r := range m.Rules {
			if r.FixedTarget < 0 || r.Rate < 0 {
				return errors.New("xray/sampling: all rules must have non-negative values for fixed_target and rate")
			}
			if r.ServiceName != "" || r.Host == "" || r.HTTPMethod == "" || r.URLPath == "" {
				return errors.New("xray/sampling: all non-default rules must have values for url_path, service_name, and http_method")
			}
		}
	case 2:
		for _, r := range m.Rules {
			if r.FixedTarget < 0 || r.Rate < 0 {
				return errors.New("xray/sampling: all rules must have non-negative values for fixed_target and rate")
			}
			if r.Host != "" && r.ServiceName == "" || r.HTTPMethod == "" || r.URLPath == "" {
				return errors.New("xray/sampling: all non-default rules must have values for host, url_path, service_name, and http_method")
			}
		}
	default:
		panic("do not pass")
	}
	return nil
}

// Copy returns deep copy of the manifest.
func (m *Manifest) Copy() *Manifest {
	defaultRule := *m.Default
	rules := make([]*Rule, 0, len(m.Rules))
	for _, r := range m.Rules {
		r := *r
		rules = append(rules, &r)
	}
	return &Manifest{
		Version: m.Version,
		Default: &defaultRule,
		Rules:   rules,
	}
}

func (m *Manifest) normalize() {
	if m.Version == 2 {
		return
	}
	m.Version = 2
	for _, r := range m.Rules {
		// service_name is renamed to host
		r.Host = r.ServiceName
		r.ServiceName = ""
	}
}

// Match returns whether the sampling rule matches against given parameters.
func (r *Rule) Match(req *Request) bool {
	if req == nil {
		return true
	}
	return (req.Host == "" || WildcardMatchCaseInsensitive(r.Host, req.Host)) &&
		(req.URL == "" || WildcardMatchCaseInsensitive(r.URLPath, req.URL)) &&
		(req.Method == "" || WildcardMatchCaseInsensitive(r.HTTPMethod, req.Method))
}
