package xray

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"sync"
	"time"
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
	tcp string
	udp string

	pool sync.Pool

	mu   sync.Mutex
	conn net.Conn
}

// New returns a new Client.
func New(config *Config) *Client {
	p := config.daemonEndpoints()
	client := &Client{
		tcp: p.TCP,
		udp: p.UDP,
		pool: sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
	}
	return client
}

// Emit sends seg to X-Ray daemon.
func (c *Client) Emit(ctx context.Context, seg *Segment) {
	buf := c.pool.Get().(*bytes.Buffer)
	defer c.pool.Put(buf)
	buf.Reset()
	buf.Write(header)
	enc := json.NewEncoder(buf)
	data := seg.serialize()
	if err := enc.Encode(data); err != nil {
		Errorf(ctx, "failed to encode: %v", err)
		return
	}

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
