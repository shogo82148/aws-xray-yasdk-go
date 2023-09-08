// Package whitelist provides whitelist of parameters for aws api.
package whitelist

import (
	"encoding/json"
	"io"
	"os"
)

// Whitelist is whitelist of parameters for aws api.
type Whitelist struct {
	Services map[string]*Service `json:"services"`
}

// Service is whitelist for specific aws service.
type Service struct {
	Operations map[string]*Operation `json:"operations"`
}

// Operation is whitelist of parameters for specific aws operation.
type Operation struct {
	RequestDescriptors  map[string]*Descriptor `json:"request_descriptors,omitempty"`
	RequestParameters   []string               `json:"request_parameters,omitempty"`
	ResponseDescriptors map[string]*Descriptor `json:"response_descriptors,omitempty"`
	ResponseParameters  []string               `json:"response_parameters,omitempty"`
}

// Descriptor is a rule for recording the parameter.
type Descriptor struct {
	Map      bool   `json:"map,omitempty"`
	GetKeys  bool   `json:"get_keys"`
	List     bool   `json:"list,omitempty"`
	GetCount bool   `json:"get_count,omitempty"`
	RenameTo string `json:"rename_to,omitempty"`
}

// Read reads a whitelist from r.
func Read(r io.Reader) (*Whitelist, error) {
	var list *Whitelist
	dec := json.NewDecoder(r)
	if err := dec.Decode(&list); err != nil {
		return nil, err
	}
	return list, nil
}

// ReadFile reads a whitelist from the file name.
func ReadFile(name string) (*Whitelist, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Read(f)
}
