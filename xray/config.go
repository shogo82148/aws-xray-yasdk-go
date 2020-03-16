package xray

import (
	"os"
	"strings"
	"unicode"

	"github.com/shogo82148/aws-xray-yasdk-go/xray/sampling"
)

// Config is a configure for connecting AWS X-Ray daemon
type Config struct {
	// DaemonAddress is the address for connecting AWS X-Ray daemon.
	// Its overwrites the address from AWS_XRAY_DAEMON_ADDRESS environment value.
	// By default, the SDK uses 127.0.0.1:2000 for both trace data (UDP) and sampling (TCP).
	// The format is "address:port" or "tcp:address:port udp:address:port".
	DaemonAddress string

	StreamingStrategy StreamingStrategy
	SamplingStrategy  sampling.Strategy

	// TODO: @shogo82148
	// AWS_XRAY_TRACING_NAME
	// AWS_XRAY_CONTEXT_MISSING
}

type daemonEndpoints struct {
	TCP string
	UDP string
}

func (c *Config) daemonEndpoints() daemonEndpoints {
	var addr string
	if c != nil {
		addr = c.DaemonAddress
	} else {
		addr = os.Getenv("AWS_XRAY_DAEMON_ADDRESS")
	}

	p := daemonEndpoints{
		TCP: "127.0.0.1:2000",
		UDP: "127.0.0.1:2000",
	}

	for {
		// split by `\s+`
		addr = strings.TrimSpace(addr)
		if addr == "" {
			break
		}
		endpoint := addr
		if idx := strings.IndexFunc(addr, unicode.IsSpace); idx >= 0 {
			endpoint = addr[:idx]
			addr = addr[idx:]
		} else {
			addr = ""
		}

		switch {
		case strings.HasPrefix(endpoint, "tcp:"):
			p.TCP = endpoint[len("tcp:"):]
		case strings.HasPrefix(endpoint, "udp:"):
			p.UDP = endpoint[len("udp:"):]
		default:
			p.TCP = endpoint
			p.UDP = endpoint
		}
	}
	return p
}
