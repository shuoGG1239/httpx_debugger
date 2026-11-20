package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Proxy 是 HTTP/HTTPS MITM 代理核心。
// - 明文 HTTP：客户端发 absolute-form 请求，直接转发并记录。
// - HTTPS：CONNECT 后对客户端扮演 TLS 服务端（出示目标主机叶子证书），
//   解密明文后与上游再建 TLS 转发，两侧明文都记录。
// - 拦截模式：命中规则（host 包含）的请求/响应转发前暂停，交 UI 裁决。
// - WinDivert 全流量模式：上游连接经 engine 注册白名单，避免被透明重定向环路捕获。
type Proxy struct {
	ca      *CA
	store   *SessionStore
	maxBody int

	server      *http.Server
	ln          net.Listener
	idSeq       atomic.Int64
	interceptor *Interceptor
	engine      *RedirectEngine

	upstream *http.Transport
}

func NewProxy(ca *CA, store *SessionStore, maxBody int, interceptor *Interceptor, engine *RedirectEngine) *Proxy {
	tr := &http.Transport{
		Proxy: nil, // 直连上游，不套任何代理
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return newProxyDialer().DialContext(ctx, network, addr)
		},
		ForceAttemptHTTP2:   false,
		TLSNextProto:        map[string]func(string, *tls.Conn) http.RoundTripper{}, // 关闭上游 h2，保持记录简单
		MaxIdleConns:        128,
		MaxIdleConnsPerHost: 16,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	p := &Proxy{ca: ca, store: store, maxBody: maxBody, interceptor: interceptor, engine: engine, upstream: tr}
	if engine != nil {
		tr.DialContext = p.upstreamDial
	}
	return p
}

func newProxyDialer() *net.Dialer {
	return &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
}

// upstreamDial 建立到上游的连接；WinDivert 模式开启时注册白名单防环路。
func (p *Proxy) upstreamDial(ctx context.Context, network, addr string) (net.Conn, error) {
	conn, err := newProxyDialer().DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	if p.engine != nil {
		release := registerConnWhitelist(p.engine, conn)
		conn = &whitelistedConn{Conn: conn, release: release}
	}
	return conn, nil
}

func registerConnWhitelist(e *RedirectEngine, conn net.Conn) func() {
	local, lok := conn.LocalAddr().(*net.TCPAddr)
	remote, rok := conn.RemoteAddr().(*net.TCPAddr)
	if !lok || !rok {
		return func() {}
	}
	return e.RegisterProxyConn(local.IP.String(), uint16(local.Port), remote.IP.String(), uint16(remote.Port))
}

// whitelistedConn 关闭时释放代理连接白名单条目。
type whitelistedConn struct {
	net.Conn
	release func()
	once    atomic.Bool
}

func (c *whitelistedConn) Close() error {
	if c.once.CompareAndSwap(false, true) {
		c.release()
	}
	return c.Conn.Close()
}

func (p *Proxy) Start(listen string) error {
	if p.server != nil {
		return errors.New("proxy already started")
	}
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	p.ln = ln
	srv := &http.Server{Handler: p, ReadHeaderTimeout: 30 * time.Second}
	p.server = srv
	go func() {
		_ = srv.Serve(ln)
	}()
	return nil
}

func (p *Proxy) Stop() {
	if p.server == nil {
		return
	}
	srv := p.server
	p.server = nil
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	_ = srv.Shutdown(ctx)
	cancel()
	_ = p.ln.Close()
	p.ln = nil
}

func (p *Proxy) ListenAddr() string {
	if p.ln != nil {
		return p.ln.Addr().String()
	}
	return ""
}

func (p *Proxy) newID() string {
	n := p.idSeq.Add(1)
	return fmt.Sprintf("s%06d", n)
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.handleConnect(w, r)
		return
	}
	if !r.URL.IsAbs() {
		http.Error(w, "expect absolute-form request URI", http.StatusBadRequest)
		return
	}
	p.relay(w, r, r.URL.Scheme, r.Host, r.URL)
}

// handleConnect 处理 CONNECT：先建隧道，再尝试对客户端做 TLS 握手进入 MITM。
func (p *Proxy) handleConnect(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	addr := host
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = host + ":443"
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking unsupported", http.StatusInternalServerError)
		return
	}
	clientConn, buf, err := hj.Hijack()
	if err != nil {
		return
	}
	if _, err := clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		clientConn.Close()
		return
	}

	if p.serveTLSClient(clientConn, buf.Reader, host) {
		return
	}
	// 非 HTTP 隧道（SSH/其他）：原样透传字节，记录为 tunnel。
	p.tunnelRaw(host, addr, clientConn, buf.Reader)
}

// serveTLSClient 对一条已建立的客户端 TCP 连接扮演 TLS 服务端（MITM）。
// host 用于签发叶子证书与构造上游 URL；握手/证书失败返回 false（连接已关闭）。
// 供 CONNECT（handleConnect）与 WinDivert 透明连接（transparent.go）复用。
func (p *Proxy) serveTLSClient(clientConn net.Conn, buf *bufio.Reader, host string) bool {
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}
	leaf, err := p.ca.LeafForHost(hostname)
	if err != nil {
		clientConn.Close()
		return false
	}
	cfg := &tls.Config{
		Certificates: []tls.Certificate{*leaf},
		NextProtos:   []string{"http/1.1"},
	}
	tlsConn := tls.Server(&bufferedConn{Conn: clientConn, r: buf}, cfg)
	_ = tlsConn.SetDeadline(time.Now().Add(10 * time.Second))
	if err := tlsConn.Handshake(); err != nil {
		log.Printf("MITM handshake failed for %s: %v", host, err)
		tlsConn.Close()
		return false
	}
	_ = tlsConn.SetDeadline(time.Time{})
	inner := &http.Server{
		Handler: http.HandlerFunc(func(w2 http.ResponseWriter, r2 *http.Request) {
			target := &url.URL{
				Scheme:   "https",
				Host:     host,
				Path:     r2.URL.Path,
				RawQuery: r2.URL.RawQuery,
			}
			p.relay(w2, r2, "https", host, target)
		}),
		ReadHeaderTimeout: 30 * time.Second,
	}
	done := make(chan struct{})
	var ln *singleConnListener
	notify := &notifyCloseConn{Conn: tlsConn, onClose: func() { ln.notifyClosed() }}
	ln = &singleConnListener{conn: notify, done: done}
	_ = inner.Serve(ln)
	return true
}

// bufferedConn 让 Hijack 后可能残留缓冲的 Reader 优先被消费。
type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.r.Read(p) }

// singleConnListener 把一条已建立的连接伪装成 Listener，供 http.Server.Serve 消费。
// 首次 Accept 返回连接；之后阻塞直到连接关闭（由 close 通知触发），Serve 才返回——
// 这样 keep-alive 连接上的多个请求都能被同一 inner server 处理。
type singleConnListener struct {
	conn net.Conn
	mu   sync.Mutex
	used bool
	done chan struct{}
	once sync.Once
}

func (l *singleConnListener) Accept() (net.Conn, error) {
	l.mu.Lock()
	if l.used {
		l.mu.Unlock()
		<-l.done
		return nil, net.ErrClosed
	}
	l.used = true
	l.mu.Unlock()
	return l.conn, nil
}

// notifyClosed 由连接关闭回调触发，只允许关闭一次 done。
func (l *singleConnListener) notifyClosed() { l.once.Do(func() { close(l.done) }) }

func (l *singleConnListener) Close() error {
	l.notifyClosed()
	return nil
}

func (l *singleConnListener) Addr() net.Addr { return l.conn.LocalAddr() }

// notifyCloseConn 在连接被关闭时触发一次回调（用于唤醒 singleConnListener 的 Accept）。
type notifyCloseConn struct {
	net.Conn
	once    atomic.Bool
	onClose func()
}

func (c *notifyCloseConn) Close() error {
	if c.once.CompareAndSwap(false, true) {
		c.onClose()
	}
	return c.Conn.Close()
}

func (p *Proxy) tunnelRaw(host, addr string, client net.Conn, buf *bufio.Reader) {
	up, err := p.upstreamDial(context.Background(), "tcp", addr)
	if err != nil {
		client.Close()
		return
	}
	sess := &Session{
		ID:        p.newID(),
		StartedAt: time.Now().UnixMilli(),
		Scheme:    "tunnel",
		Method:    "CONNECT",
		Host:      host,
		URL:       "tunnel://" + host,
		Error:     "non-HTTP tunnel (raw relay)",
	}
	go func() {
		_, _ = io.Copy(up, buf)
		if tc, ok := up.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
	}()
	_, _ = io.Copy(client, up)
	_ = up.Close()
	_ = client.Close()
	sess.ElapsedMs = time.Now().UnixMilli() - sess.StartedAt
	p.store.Add(sess)
}

// relay 处理一个已解析的 HTTP 请求/响应对：可选拦截 → 转发上游 → 可选响应拦截 → 写回客户端，全程记录明文。
func (p *Proxy) relay(w http.ResponseWriter, r *http.Request, scheme, host string, target *url.URL) {
	start := time.Now()
	sess := &Session{
		ID:         p.newID(),
		StartedAt:  start.UnixMilli(),
		Scheme:     scheme,
		Method:     r.Method,
		Host:       host,
		URL:        target.String(),
		ClientAddr: r.RemoteAddr,
	}

	reqBody, reqTrunc, _ := readBodyCapped(r.Body, p.maxBody)
	sess.RequestBody = reqBody
	sess.RequestTruncated = reqTrunc
	sess.RequestContentType = r.Header.Get("Content-Type")
	sess.RequestHeaders = flattenHeaders(r.Header)

	method := r.Method
	reqHeaders := sess.RequestHeaders
	reqBodyOut := reqBody

	// ---- 请求拦截 ----
	if p.interceptor != nil && p.interceptor.shouldIntercept("request", host) {
		if d, err := p.interceptor.InterceptRequest(sess.ID, method, target.String(), reqHeaders, reqBody); err == nil {
			switch d.Action {
			case "drop":
				sess.Status = http.StatusForbidden
				sess.StatusText = "403 Forbidden"
				sess.Error = "request dropped by user"
				sess.ElapsedMs = time.Since(start).Milliseconds()
				p.store.Add(sess)
				http.Error(w, "request dropped by interceptor", http.StatusForbidden)
				return
			case "continue":
				if d.Method != "" {
					method = d.Method
				}
				if d.URL != "" {
					if u, perr := url.Parse(d.URL); perr == nil && u.IsAbs() {
						target = u
					}
				}
				if d.Headers != nil {
					reqHeaders = d.Headers
				}
				if d.Body != nil {
					reqBodyOut = d.Body
				}
				sess.Method = method
				sess.URL = target.String()
				sess.RequestHeaders = reqHeaders
				sess.RequestBody = reqBodyOut
			}
		}
		// 超时 → 原样继续
	}

	upReq := buildUpstreamRequest(method, target, host, reqHeaders, reqBodyOut, r.RemoteAddr)
	res := p.doUpstream(sess, upReq)
	if res.err != nil {
		sess.ElapsedMs = time.Since(start).Milliseconds()
		p.store.Add(sess)
		http.Error(w, "upstream error: "+res.err.Error(), http.StatusBadGateway)
		return
	}

	status := res.status
	statusText := res.statusText
	outHeaders := res.headers
	outBody := res.bodyRaw
	bodyModified := false

	// ---- 响应拦截 ----
	if p.interceptor != nil && p.interceptor.shouldIntercept("response", host) {
		if d, err := p.interceptor.InterceptResponse(sess.ID, status, statusText, outHeaders, res.body); err == nil {
			switch d.Action {
			case "drop":
				sess.Error = "response dropped by user"
				sess.ElapsedMs = time.Since(start).Milliseconds()
				p.store.Add(sess)
				return // 不向客户端写任何字节
			case "continue":
				if d.Status != 0 {
					status = d.Status
					statusText = d.StatusText
				}
				if d.Headers != nil {
					outHeaders = d.Headers
				}
				if d.Body != nil {
					outBody = d.Body
					bodyModified = true
				}
				sess.Status = status
				sess.StatusText = statusText
				sess.ResponseHeaders = outHeaders
				sess.ResponseBody = outBody
			}
		}
		// 超时 → 原样写回
	}

	sess.ElapsedMs = time.Since(start).Milliseconds()
	p.store.Add(sess)

	// 写回客户端：跳过逐跳头与 Content-Length（交给 net/http 自动计算，截断时不冲突）
	outH := w.Header()
	for _, h := range outHeaders {
		if isHopByHopName(h.Name) || strings.EqualFold(h.Name, "Content-Length") {
			continue
		}
		if bodyModified && strings.EqualFold(h.Name, "Content-Encoding") {
			continue // body 已被 UI 改过，按明文写回
		}
		outH.Add(h.Name, h.Value)
	}
	w.WriteHeader(status)
	_, _ = w.Write(outBody)
}

// buildUpstreamRequest 用（可能被拦截修改后的）请求构造上游请求。
func buildUpstreamRequest(method string, target *url.URL, host string, headers []Header, body []byte, remoteAddr string) *http.Request {
	upReq := &http.Request{
		Method:        method,
		URL:           target,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        make(http.Header),
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Host:          host,
		RemoteAddr:    remoteAddr,
		RequestURI:    "",
	}
	for _, h := range headers {
		if isHopByHopName(h.Name) {
			continue
		}
		upReq.Header.Add(h.Name, h.Value)
	}
	return upReq.WithContext(context.Background())
}

// upstreamResult 携带上游响应（同时已记录到 sess 响应字段）。
type upstreamResult struct {
	err error

	status      int
	statusText  string
	headers     []Header // 扁平化排序后的响应头
	body        []byte   // 已解压（记录/编辑用）
	bodyRaw     []byte   // 原始字节（写回客户端用）
	truncated   bool
	contentType string
}

// doUpstream 执行上游 RoundTrip 并填充 sess 响应字段（不 Add，由调用方决定时机）。
func (p *Proxy) doUpstream(sess *Session, upReq *http.Request) *upstreamResult {
	resp, err := p.upstream.RoundTrip(upReq)
	sess.ElapsedMs = time.Now().UnixMilli() - sess.StartedAt
	if err != nil {
		sess.Error = err.Error()
		return &upstreamResult{err: err}
	}
	defer resp.Body.Close()

	respRaw, trunc, _ := readBodyCapped(resp.Body, p.maxBody)
	decoded := decodeBody(respRaw, resp.Header.Get("Content-Encoding"))
	if len(decoded) > maxDecodedSize {
		decoded = decoded[:maxDecodedSize]
		trunc = true
	}
	sess.Status = resp.StatusCode
	sess.StatusText = resp.Status
	sess.ResponseContentType = resp.Header.Get("Content-Type")
	sess.ResponseBody = decoded
	sess.ResponseTruncated = trunc
	sess.ResponseHeaders = flattenHeaders(resp.Header)
	return &upstreamResult{
		status:      resp.StatusCode,
		statusText:  resp.Status,
		headers:     sess.ResponseHeaders,
		body:        decoded,
		bodyRaw:     respRaw,
		truncated:   trunc,
		contentType: resp.Header.Get("Content-Type"),
	}
}

// Replay 重新发送一个请求（UI 重放），记录为新会话；不经过拦截。
func (p *Proxy) Replay(method, rawURL string, headers []Header, body []byte) error {
	target, err := url.Parse(rawURL)
	if err != nil || !target.IsAbs() {
		return errors.New("invalid absolute url: " + rawURL)
	}
	if method == "" {
		method = http.MethodGet
	}
	host := target.Host
	start := time.Now()
	sess := &Session{
		ID:         p.newID(),
		StartedAt:  start.UnixMilli(),
		Scheme:     target.Scheme,
		Method:     method,
		Host:       host,
		URL:        target.String(),
		ClientAddr: "replay",
	}
	reqHeaders := headers
	if reqHeaders == nil {
		reqHeaders = []Header{}
	}
	sess.RequestHeaders = reqHeaders
	sess.RequestBody = body
	sess.RequestContentType = headerValue(reqHeaders, "Content-Type")
	sess.RequestTruncated = len(body) > p.maxBody
	if sess.RequestTruncated {
		sess.RequestBody = body[:p.maxBody]
	}

	upReq := buildUpstreamRequest(method, target, host, reqHeaders, sess.RequestBody, "replay")
	res := p.doUpstream(sess, upReq)
	sess.ElapsedMs = time.Now().UnixMilli() - sess.StartedAt
	p.store.Add(sess)
	return res.err
}

func headerValue(headers []Header, name string) string {
	for _, h := range headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}

func flattenHeaders(h http.Header) []Header {
	out := make([]Header, 0, len(h))
	for k, vs := range h {
		for _, v := range vs {
			out = append(out, Header{Name: k, Value: v})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

var hopByHopHeaders = []string{
	"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization",
	"Proxy-Connection", "TE", "Trailer", "Transfer-Encoding", "Upgrade",
}

func isHopByHopName(name string) bool {
	for _, k := range hopByHopHeaders {
		if strings.EqualFold(k, name) {
			return true
		}
	}
	return false
}
