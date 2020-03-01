package xray

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net"
	"sync"
	"time"

	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

var header = []byte(`{"format":"json","version":1}` + "\n")
var dialer = net.Dialer{
	Timeout: 100 * time.Millisecond,
}

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
	data := schema.Segment{
		// TODO: @shogo82148 fill correct data
		Name:      "test test",
		ID:        "70de5b6f19ff9a0a",
		TraceID:   NewTraceID(),
		StartTime: float64(time.Now().UnixNano()) / 1e9,
		EndTime:   float64(time.Now().Add(time.Second).UnixNano()) / 1e9,
	}
	if err := enc.Encode(data); err != nil {
		// TODO: @shogo82148 log
		log.Println(err)
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		conn, err := dialer.DialContext(context.Background(), "udp", c.addr)
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
