package envconfig

import (
	"os"
	"strconv"
	"time"
)

// MetadataServiceTimeout returns the timeout for the metadata service.
func MetadataServiceTimeout() time.Duration {
	const defaultTimeout = time.Second

	timeout := os.Getenv("AWS_METADATA_SERVICE_TIMEOUT")
	if timeout == "" {
		return defaultTimeout
	}
	d, err := strconv.ParseInt(timeout, 10, 64)
	if err != nil {
		return defaultTimeout
	}
	if d <= 0 {
		return defaultTimeout
	}
	return time.Duration(d) * time.Second
}

// MetadataServiceNumAttempts returns the number of attempts to make when
// querying the metadata service.
func MetadataServiceNumAttempts() int {
	const defaultNumAttempts = 1

	attempts := os.Getenv("AWS_METADATA_SERVICE_NUM_ATTEMPTS")
	if attempts == "" {
		return defaultNumAttempts
	}
	n, err := strconv.ParseInt(attempts, 10, 0)
	if err != nil {
		return defaultNumAttempts
	}
	if n < 1 {
		return defaultNumAttempts
	}
	return int(n)
}
