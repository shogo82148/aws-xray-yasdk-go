package xray

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"sync"
	"time"

	"github.com/shogo82148/aws-xray-yasdk-go/xray/sampling"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

const emitTimeout = 100 * time.Millisecond

var header = []byte(`{"format":"json","version":1}` + "\n")
var dialer = net.Dialer{
	Timeout: emitTimeout,
}

var defaultClient = New(nil)

// Client is a client for AWS X-Ray daemon.
type Client struct {
	// the address of the AWS X-Ray daemon
	udp string

	pool sync.Pool

	streamingStrategy StreamingStrategy
	samplingStrategy  sampling.Strategy

	mu   sync.Mutex
	conn net.Conn
}

// New returns a new Client.
func New(config *Config) *Client {
	// initialize sampling strategy
	p := config.daemonEndpoints()
	var samplingStrategy sampling.Strategy
	if config != nil {
		samplingStrategy = config.SamplingStrategy
	}
	if samplingStrategy == nil {
		var err error
		samplingStrategy, err = sampling.NewCentralizedStrategy(p.TCP, nil)
		if err != nil {
			panic(err)
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
		streamingStrategy: streamingStrategy,
		samplingStrategy:  samplingStrategy,
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
	buf := c.pool.Get().(*bytes.Buffer)
	defer c.pool.Put(buf)
	buf.Reset()
	buf.Write(header)
	enc := json.NewEncoder(buf)
	if err := enc.Encode(seg); err != nil {
		Errorf(ctx, "failed to encode: %v", err)
		return
	}
	Debugf(ctx, "emit: %s", buf.Bytes()[len(header):])

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		emitCtx, cancel := context.WithTimeout(context.Background(), emitTimeout)
		defer cancel()

		conn, err := dialer.DialContext(emitCtx, "udp", c.udp)
		if err != nil {
			Errorf(ctx, "failed to dial: %v", err)
			return
		}
		c.conn = conn
	}
	if _, err := c.conn.Write(buf.Bytes()); err != nil {
		Errorf(ctx, "failed to write: %v", err)
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
