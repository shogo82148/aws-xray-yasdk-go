package xrayhttp

import (
	"context"
	"crypto/tls"
	"net/http/httptrace"
	"net/textproto"
	"sync"

	"github.com/shogo82148/aws-xray-yasdk-go/xray"
)

type httpSubsegments struct {
	mu      sync.Mutex
	ctx     context.Context
	connCtx context.Context
	connSeg *xray.Segment
	dnsCtx  context.Context
	dnsSeg  *xray.Segment
	dialCtx context.Context
	dialSeg *xray.Segment
	tlsCtx  context.Context
	tlsSeg  *xray.Segment
	reqCtx  context.Context
	reqSeg  *xray.Segment
}

func (segs *httpSubsegments) GetConn(hostPort string) {
	segs.mu.Lock()
	defer segs.mu.Unlock()
	segs.connCtx, segs.connSeg = xray.BeginSubsegment(segs.ctx, "connect")
}

func (segs *httpSubsegments) GotConn(info httptrace.GotConnInfo) {
	segs.mu.Lock()
	defer segs.mu.Unlock()
	if segs.connCtx != nil {
		segs.connSeg.Close()
		segs.connCtx, segs.connSeg = nil, nil
	}

	segs.reqCtx, segs.reqSeg = xray.BeginSubsegment(segs.ctx, "request")
}

func (segs *httpSubsegments) PutIdleConn(err error)                                      {}
func (segs *httpSubsegments) GotFirstResponseByte()                                      {}
func (segs *httpSubsegments) Got100Continue()                                            {}
func (segs *httpSubsegments) Got1xxResponse(code int, header textproto.MIMEHeader) error { return nil }

func (segs *httpSubsegments) DNSStart(info httptrace.DNSStartInfo) {
	segs.mu.Lock()
	defer segs.mu.Unlock()
	if segs.connCtx == nil {
		return
	}
	segs.dnsCtx, segs.dnsSeg = xray.BeginSubsegment(segs.connCtx, "dns")
}

func (segs *httpSubsegments) DNSDone(info httptrace.DNSDoneInfo) {
	type dnsDoneInfo struct {
		Addresses []string `json:"addresses"`
		Coalesced bool     `json:"coalesced"`
	}

	segs.mu.Lock()
	defer segs.mu.Unlock()
	if segs.dnsCtx == nil {
		return
	}
	addresses := make([]string, 0, len(info.Addrs))
	for _, addr := range info.Addrs {
		addresses = append(addresses, addr.String())
	}
	meta := dnsDoneInfo{
		Addresses: addresses,
		Coalesced: info.Coalesced,
	}
	segs.dnsSeg.AddMetadataToNamespace("http", "dns", meta)
	segs.dnsSeg.AddError(info.Err)
	segs.dnsSeg.Close()
	segs.dnsCtx, segs.dnsSeg = nil, nil
}

func (segs *httpSubsegments) ConnectStart(network, addr string) {
	segs.mu.Lock()
	defer segs.mu.Unlock()
	if segs.connCtx == nil {
		return
	}
	segs.dialCtx, segs.dialSeg = xray.BeginSubsegment(segs.connCtx, "dial")
}
func (segs *httpSubsegments) ConnectDone(network, addr string, err error) {
	segs.mu.Lock()
	defer segs.mu.Unlock()
	if segs.dialCtx == nil {
		return
	}
	segs.dialSeg.AddError(err)
	segs.dialSeg.Close()
	segs.dialCtx, segs.dialSeg = nil, nil
}

func (segs *httpSubsegments) TLSHandshakeStart() {
	segs.mu.Lock()
	defer segs.mu.Unlock()
	if segs.connCtx == nil {
		return
	}
	segs.tlsCtx, segs.tlsSeg = xray.BeginSubsegment(segs.connCtx, "tls")
}

func (segs *httpSubsegments) TLSHandshakeDone(tls.ConnectionState, error) {
	segs.mu.Lock()
	defer segs.mu.Unlock()
	if segs.tlsCtx == nil {
		return
	}
	segs.tlsSeg.Close()
	segs.tlsCtx, segs.tlsSeg = nil, nil
}

func (segs *httpSubsegments) WroteHeaderField(key string, value []string) {}
func (segs *httpSubsegments) WroteHeaders()                               {}
func (segs *httpSubsegments) Wait100Continue()                            {}

func (segs *httpSubsegments) WroteRequest(httptrace.WroteRequestInfo) {
	segs.mu.Lock()
	defer segs.mu.Unlock()
	if segs.reqCtx != nil {
		segs.reqSeg.Close()
		segs.reqCtx, segs.reqSeg = nil, nil
	}
}

type clientTrace struct {
	segments  *httpSubsegments
	httptrace *httptrace.ClientTrace
}

// WithClientTrace returns a new context based on the provided parent ctx.
func WithClientTrace(ctx context.Context) context.Context {
	if ctx == nil {
		panic("ctx must not be nil")
	}
	segs := &httpSubsegments{
		ctx: ctx,
	}
	trace := &clientTrace{
		segments: segs,
		httptrace: &httptrace.ClientTrace{
			GetConn:              segs.GetConn,
			GotConn:              segs.GotConn,
			PutIdleConn:          segs.PutIdleConn,
			GotFirstResponseByte: segs.GotFirstResponseByte,
			Got100Continue:       segs.Got100Continue,
			Got1xxResponse:       segs.Got1xxResponse,
			DNSStart:             segs.DNSStart,
			DNSDone:              segs.DNSDone,
			ConnectStart:         segs.ConnectStart,
			ConnectDone:          segs.ConnectDone,
			TLSHandshakeStart:    segs.TLSHandshakeStart,
			TLSHandshakeDone:     segs.TLSHandshakeDone,
			WroteHeaderField:     segs.WroteHeaderField,
			WroteHeaders:         segs.WroteHeaders,
			Wait100Continue:      segs.Wait100Continue,
			WroteRequest:         segs.WroteRequest,
		},
	}
	return httptrace.WithClientTrace(ctx, trace.httptrace)
}
