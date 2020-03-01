package xray

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net"
	"sync"
	"time"
)

const emitTimeout = 100 * time.Millisecond

var header = []byte(`{"format":"json","version":1}` + "\n")
var dialer = net.Dialer{
	Timeout: emitTimeout,
}

var defaultClient = New("127.0.0.1:2000")

// Client is a client for X-Ray daemon.
type Client struct {
	mu   sync.Mutex
	addr string
	conn *net.UDPConn
	pool sync.Pool
}

// New returns a new Client.
func New(addr string) *Client {
	return &Client{
		addr: addr,
		pool: sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
	}
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
		// TODO: @shogo82148 log
		log.Println(err)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		emitCtx, cancel := context.WithTimeout(context.Background(), emitTimeout)
		defer cancel()

		conn, err := dialer.DialContext(emitCtx, "udp", c.addr)
		if err != nil {
			// TODO: @shogo82148 log
			log.Println(err)
			return
		}
		c.conn = conn.(*net.UDPConn)
	}
	if _, err := c.conn.Write(buf.Bytes()); err != nil {
		// TODO: @shogo82148 log
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
