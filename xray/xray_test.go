package xray

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/shogo82148/aws-xray-yasdk-go/xray/schema"
)

type TestDaemon struct {
	ch        <-chan *result
	conn      net.PacketConn
	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
}

func NewTestDaemon() (context.Context, *TestDaemon) {
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
	ctx = context.WithValue(ctx, clientContextKey, New(&Config{
		DaemonAddress: conn.LocalAddr().String(),
	}))

	go d.run(c)
	return ctx, d
}

type result struct {
	Segment *schema.Segment
	Error   error
}

func (td *TestDaemon) Close() {
	td.closeOnce.Do(func() {
		td.cancel()
		td.conn.Close()
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
