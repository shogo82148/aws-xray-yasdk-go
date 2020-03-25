package xrayhttp

import (
	"context"
	"crypto/tls"
	"fmt"
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

	if info.Err != nil && segs.connCtx != nil {
		segs.connSeg.SetFault()
		segs.connSeg.Close()
		segs.connCtx, segs.connSeg = nil, nil
	}
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
	type dialInfo struct {
		Network string `json:"network"`
		Address string `json:"address"`
	}

	segs.mu.Lock()
	defer segs.mu.Unlock()
	if segs.dialCtx == nil {
		return
	}
	segs.dialSeg.AddMetadataToNamespace("http", "dial", dialInfo{
		Network: network,
		Address: addr,
	})
	segs.dialSeg.AddError(err)
	segs.dialSeg.Close()
	segs.dialCtx, segs.dialSeg = nil, nil

	if err != nil && segs.connCtx != nil {
		segs.connSeg.SetFault()
		segs.connSeg.Close()
		segs.connCtx, segs.connSeg = nil, nil
	}
}

func (segs *httpSubsegments) TLSHandshakeStart() {
	segs.mu.Lock()
	defer segs.mu.Unlock()
	if segs.connCtx == nil {
		return
	}
	segs.tlsCtx, segs.tlsSeg = xray.BeginSubsegment(segs.connCtx, "tls")
}

func (segs *httpSubsegments) TLSHandshakeDone(state tls.ConnectionState, err error) {
	type tlsInfo struct {
		Version                    string `json:"version,omitempty"`
		DidResume                  bool   `json:"did_resume,omitempty"`
		NegotiatedProtocol         string `json:"negotiated_protocol,omitempty"`
		NegotiatedProtocolIsMutual bool   `json:"negotiated_protocol_is_mutual,omitempty"`
		CipherSuite                string `json:"cipher_suite,omitempty"`
	}

	segs.mu.Lock()
	defer segs.mu.Unlock()
	if segs.tlsCtx == nil {
		return
	}
	if !segs.tlsSeg.AddError(err) {
		segs.tlsSeg.AddMetadataToNamespace("http", "tls", tlsInfo{
			Version:                    tlsVersionName(state.Version),
			DidResume:                  state.DidResume,
			NegotiatedProtocol:         state.NegotiatedProtocol,
			NegotiatedProtocolIsMutual: state.NegotiatedProtocolIsMutual,
			CipherSuite:                cipherSuiteName(state.CipherSuite),
		})
	}
	segs.tlsSeg.Close()
	segs.tlsCtx, segs.tlsSeg = nil, nil

	if err != nil && segs.connCtx != nil {
		segs.connSeg.SetFault()
		segs.connSeg.Close()
		segs.connCtx, segs.connSeg = nil, nil
	}
}

func tlsVersionName(version uint16) string {
	switch version {
	case tls.VersionSSL30:
		return "ssl3.0"
	case tls.VersionTLS10:
		return "tls1.0"
	case tls.VersionTLS11:
		return "tls1.1"
	case tls.VersionTLS12:
		return "tls1.2"
	case 0x0304: // VersionTLS13 is supported from Go 1.13
		return "tls1.3"
	}

	// fallback to hex format
	return fmt.Sprintf("0x%04X", version)
}

// naive implementation of tls.CipherSuiteName from Go 1.14
func cipherSuiteName(id uint16) string {
	switch id {
	case 0x0005:
		return "TLS_RSA_WITH_RC4_128_SHA"
	case 0x000a:
		return "TLS_RSA_WITH_3DES_EDE_CBC_SHA"
	case 0x002f:
		return "TLS_RSA_WITH_AES_128_CBC_SHA"
	case 0x0035:
		return "TLS_RSA_WITH_AES_256_CBC_SHA"
	case 0x003c:
		return "TLS_RSA_WITH_AES_128_CBC_SHA256"
	case 0x009c:
		return "TLS_RSA_WITH_AES_128_GCM_SHA256"
	case 0x009d:
		return "TLS_RSA_WITH_AES_256_GCM_SHA384"
	case 0xc007:
		return "TLS_ECDHE_ECDSA_WITH_RC4_128_SHA"
	case 0xc009:
		return "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA"
	case 0xc00a:
		return "TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA"
	case 0xc011:
		return "TLS_ECDHE_RSA_WITH_RC4_128_SHA"
	case 0xc012:
		return "TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA"
	case 0xc013:
		return "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA"
	case 0xc014:
		return "TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA"
	case 0xc023:
		return "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256"
	case 0xc027:
		return "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256"
	case 0xc02f:
		return "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"
	case 0xc02b:
		return "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256"
	case 0xc030:
		return "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384"
	case 0xc02c:
		return "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384"
	case 0xcca8:
		return "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256"
	case 0xcca9:
		return "TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256"
	case 0x1301:
		return "TLS_AES_128_GCM_SHA256"
	case 0x1302:
		return "TLS_AES_256_GCM_SHA384"
	case 0x1303:
		return "TLS_CHACHA20_POLY1305_SHA256"
	case 0x5600:
		return "TLS_FALLBACK_SCSV"
	}
	return fmt.Sprintf("0x%04X", id)
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
