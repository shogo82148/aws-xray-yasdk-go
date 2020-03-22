package xray

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"time"

	"github.com/shogo82148/aws-xray-yasdk-go/xray/sampling"
	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

// TestDaemon is the mock server of AWS X-Ray daemon.
type TestDaemon struct {
	// ContextMissing is callback function for the context missing strategy.
	// If it is nil, ignore context missing errors.
	ContextMissing func(ctx context.Context, v interface{})

	ch        <-chan *result
	conn      net.PacketConn
	ctx       context.Context
	cancel    context.CancelFunc
	ts        *httptest.Server
	closeOnce sync.Once
}

// NewTestDaemon creates new TestDaemon
func NewTestDaemon(handler http.Handler) (context.Context, *TestDaemon) {
	c := make(chan *result, 200)
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		if conn, err = net.ListenPacket("udp6", "[::1]:0"); err != nil {
			panic(fmt.Sprintf("xray: failed to listen: %v", err))
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	d := &TestDaemon{
		ch:     c,
		conn:   conn,
		ctx:    ctx,
		cancel: cancel,
	}
	address := "udp:" + conn.LocalAddr().String()
	if handler != nil {
		d.ts = httptest.NewServer(handler)
		u, err := url.Parse(d.ts.URL)
		if err != nil {
			panic(err)
		}
		address += " tcp:" + u.Host
	}

	ctx = context.WithValue(ctx, clientContextKey, New(&Config{
		DaemonAddress:      address,
		SamplingStrategy:   sampling.NewAllStrategy(),
		CtxmissingStrategy: &testDaemonContextMissing{td: d},
	}))

	go d.run(c)
	return ctx, d
}

type testDaemonContextMissing struct {
	td *TestDaemon
}

func (s *testDaemonContextMissing) ContextMissing(ctx context.Context, v interface{}) {
	if s != nil && s.td != nil && s.td.ContextMissing != nil {
		s.td.ContextMissing(ctx, v)
	}
}

type result struct {
	Segment *schema.Segment
	Error   error
}

// Close shutdowns the daemon.
func (td *TestDaemon) Close() {
	td.closeOnce.Do(func() {
		td.cancel()
		td.conn.Close()
		if td.ts != nil {
			td.ts.Close()
		}
	})
}

func (td *TestDaemon) run(c chan *result) {
	buffer := make([]byte, 64*1024)
	for {
		n, _, err := td.conn.ReadFrom(buffer)
		if err != nil {
			select {
			case c <- &result{nil, err}:
			case <-td.ctx.Done():
				return
			}
			continue
		}

		idx := bytes.IndexByte(buffer, '\n')
		buffered := buffer[idx+1 : n]

		var seg *schema.Segment
		err = json.Unmarshal(buffered, &seg)
		if err != nil {
			select {
			case c <- &result{nil, err}:
			case <-td.ctx.Done():
				return
			}
			continue
		}

		select {
		case c <- &result{seg, nil}:
		case <-td.ctx.Done():
			return
		}
	}
}

// Recv returns the received segment.
func (td *TestDaemon) Recv() (*schema.Segment, error) {
	ctx, cancel := context.WithTimeout(td.ctx, 500*time.Millisecond)
	defer cancel()
	select {
	case r := <-td.ch:
		return r.Segment, r.Error
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
