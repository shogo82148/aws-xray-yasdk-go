package xray

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"sync"
	"time"

	"github.com/shogo82148/aws-xray-yasdk-go/xray/ctxmissing"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/sampling"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/xraylog"
)

const emitTimeout = 100 * time.Millisecond

var header = []byte(`{"format":"json","version":1}` + "\n")
var dialer = net.Dialer{
	Timeout: emitTimeout,
}

var defaultClient = New(nil)

// Configure relaces the default client with the cfg.
func Configure(cfg *Config) {
	client := New(cfg)
	defaultClient.Close()
	defaultClient = client
}

// ContextClient returns the client of current context.
func ContextClient(ctx context.Context) *Client {
	if client := ctx.Value(clientContextKey); client != nil {
		return client.(*Client)
	}
	return defaultClient
}

// WithClient returns new context with the client.
func WithClient(ctx context.Context, client *Client) context.Context {
	return context.WithValue(ctx, clientContextKey, client)
}

// Client is a client for AWS X-Ray daemon.
type Client struct {
	// the address of the AWS X-Ray daemon
	udp string

	pool sync.Pool

	disabled               bool
	streamingStrategy      StreamingStrategy
	samplingStrategy       sampling.Strategy
	contextMissingStrategy ctxmissing.Strategy

	mu   sync.Mutex
	conn net.Conn
}

// New returns a new Client.
func New(config *Config) *Client {
	// initialize sampling strategy
	p := config.daemonEndpoints()
	var samplingStrategy sampling.Strategy
	var contextMissingStrategy ctxmissing.Strategy
	if config != nil {
		samplingStrategy = config.SamplingStrategy
		contextMissingStrategy = config.ContextMissingStrategy
	}
	if samplingStrategy == nil {
		var err error
		samplingStrategy, err = sampling.NewCentralizedStrategy(p.TCP, nil)
		if err != nil {
			panic(err)
		}
	}
	if contextMissingStrategy == nil {
		switch os.Getenv("AWS_XRAY_CONTEXT_MISSING") {
		case "LOG_ERROR":
			contextMissingStrategy = &ctxmissing.LogErrorStrategy{}
		case "RUNTIME_ERROR":
			contextMissingStrategy = &ctxmissing.RuntimeErrorStrategy{}
		default:
			contextMissingStrategy = &ctxmissing.LogErrorStrategy{}
		}
	}

	// initialize streaming strategy
	streamingStrategy := NewStreamingStrategyLimitSubsegment(20)
	if config != nil && config.StreamingStrategy != nil {
		streamingStrategy = config.StreamingStrategy
	}

	client := &Client{
		udp: p.UDP,
		pool: sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
		disabled:               config.disabled(),
		streamingStrategy:      streamingStrategy,
		samplingStrategy:       samplingStrategy,
		contextMissingStrategy: contextMissingStrategy,
	}
	return client
}

// Emit sends seg to X-Ray daemon.
func (c *Client) Emit(ctx context.Context, seg *Segment) {
	for _, data := range c.streamingStrategy.StreamSegment(seg) {
		c.emit(ctx, data)
	}
}

func (c *Client) emit(ctx context.Context, seg *schema.Segment) {
	if c.disabled {
		return
	}

	buf := c.pool.Get().(*bytes.Buffer)
	defer c.pool.Put(buf)
	buf.Reset()
	buf.Write(header) // fix me
	enc := json.NewEncoder(buf)
	if err := enc.Encode(seg); err != nil {
		xraylog.Errorf(ctx, "failed to encode: %v", err)
		return
	}
	xraylog.Debugf(ctx, "emit: %s", buf.Bytes()[len(header):])

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		emitCtx, cancel := context.WithTimeout(context.Background(), emitTimeout)
		defer cancel()

		conn, err := dialer.DialContext(emitCtx, "udp", c.udp)
		if err != nil {
			xraylog.Errorf(ctx, "failed to dial: %v", err)
			return
		}
		c.conn = conn
	}
	if _, err := c.conn.Write(buf.Bytes()); err != nil {
		xraylog.Errorf(ctx, "failed to write: %v", err)
		return
	}
}

// Close closes the client.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}
