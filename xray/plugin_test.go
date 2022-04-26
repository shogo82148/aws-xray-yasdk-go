package xray

import (
	"sync"
	"testing"
)

func TestAddPlugin(t *testing.T) {
	org := getPlugins()
	defer func() {
		muPlugins.Lock()
		defer muPlugins.Unlock()
		plugins = org
	}()
	before := len(getPlugins())

	// test of races
	const n = 10
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			AddPlugin(&xrayPlugin{
				sdkVersion: getVersion(),
			})
			getPlugins()
		}()
	}
	wg.Wait()

	after := len(getPlugins())

	if after-before != n {
		t.Errorf("unexpected plugin count: want %d, got %d", n, after-before)
	}
}
