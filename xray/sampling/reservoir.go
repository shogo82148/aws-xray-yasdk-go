package sampling

import (
	"time"
	"sync"
)

type reservoir struct {
	mu sync.Mutex

	nowFunc func() time.Time

	// Total size of reservoir
	capacity int64

	// Reservoir consumption for current epoch
	used int64

	// Unix epoch. Reservoir usage is reset every second.
	currentEpoch int64
}

func (r *reservoir) Take() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.nowFunc == nil {
		r.nowFunc = time.Now
	}

	// reset counters every seconds
	if epoch := r.nowFunc().Unix(); epoch != r.currentEpoch {
		r.used = 0
		r.currentEpoch = epoch
	}

	// Take from reservoir, if available
	if r.used >= r.capacity {
		return false
	}
	r.used++
	return true
}
