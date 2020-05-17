package xray

import (
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/shogo82148/aws-xray-yasdk-go/xray/ctxmissing"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/sampling"
)

// Config is a configure for connecting AWS X-Ray daemon
type Config struct {
	// DaemonAddress is the address for connecting AWS X-Ray daemon.
	// It overwrites the address from the AWS_XRAY_DAEMON_ADDRESS environment value.
	// By default, the SDK uses 127.0.0.1:2000 for both trace data (UDP) and sampling (TCP).
	// The format is "address:port" or "tcp:address:port udp:address:port".
	DaemonAddress string

	// Disabled disables X-Ray tracing.
	// It overwrites the setting from the AWS_XRAY_SDK_ENABLED environment value.
	Disabled bool

	StreamingStrategy      StreamingStrategy
	SamplingStrategy       sampling.Strategy
	ContextMissingStrategy ctxmissing.Strategy
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

func (c *Config) disabled() bool {
	flag := os.Getenv("AWS_XRAY_SDK_ENABLED")
	if flag == "" {
		return c.Disabled
	}
	ret, err := strconv.ParseBool(flag)
	if err != nil {
		return c.Disabled
	}
	return !ret
}
