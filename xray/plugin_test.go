package xray

import (
	"sync"
	"testing"
)

func TestAddPlugin(t *testing.T) {
	before := len(getPlugins())

	// test of races
	const n = 10
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go AddPlugin(&xrayPlugin{})
		go getPlugins()
	}

	after := len(getPlugins())

	if after-before != n {
		t.Errorf("unexpected plugin count: want %d, got %d", n, after-before)
	}
}
