//go:build windows

package main

import (
	"context"
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// TransparentServer 接收 WinDivert 重定向到透明端口的连接，
// 解析出原始目标（TLS SNI / HTTP Host），走既有 MITM-relay 管线；未知协议纯隧道转发。
type TransparentServer struct {
	proxy  *Proxy
	engine *RedirectEngine

	mu      sync.Mutex
	running bool
	ln      net.Listener
	stopCh  chan struct{}
	doneCh  chan struct{}
}

func NewTransparentServer(proxy *Proxy, engine *RedirectEngine) *TransparentServer {
	return &TransparentServer{proxy: proxy, engine: engine}
}

func (t *TransparentServer) Start(port int) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.running {
		return nil
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	t.ln = ln
	t.running = true
	t.stopCh = make(chan struct{})
	t.doneCh = make(chan struct{})
	go t.acceptLoop()
	return nil
}

func (t *TransparentServer) Stop() {
	t.mu.Lock()
	if !t.running {
		t.mu.Unlock()
		return
	}
	t.running = false
	close(t.stopCh)
	ln := t.ln
	t.mu.Unlock()
	_ = ln.Close()
	select {
	case <-t.doneCh:
	case <-time.After(2 * time.Second):
	}
}

func (t *TransparentServer) Running() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.running
}

func (t *TransparentServer) acceptLoop() {
	defer close(t.doneCh)
	for {
		conn, err := t.ln.Accept()
		if err != nil {
			return
		}
		go t.handleConn(conn)
	}
}

func (t *TransparentServer) handleConn(conn net.Conn) {
	defer conn.Close()
	clientAddr, ok := conn.RemoteAddr().(*net.TCPAddr)
	if !ok {
		return
	}
	clientIP := clientAddr.IP.String()
	clientPort := uint16(clientAddr.Port)

	tgt := t.engine.lookupRedirect(clientIP, clientPort)
	if tgt == nil {
		return // 非 WinDivert 重定向来的连接
	}
	defer t.engine.DeleteRedirect(clientIP, clientPort)

	br := bufio.NewReaderSize(conn, 64*1024)
	head, err := br.Peek(8) // 覆盖最长方法 "CONNECT "（8 字节）
	if err != nil {
		return
	}

	// TLS → SNI → MITM
	if head[0] == 0x16 && head[1] == 0x03 {
		sni, err := peekSNI(br)
		if err != nil {
			t.tunnel(conn, br, tgt) // SNI 解析失败（如非标准 TLS）→ 纯隧道
			return
		}
		host := net.JoinHostPort(sni, fmt.Sprint(tgt.port))
		t.proxy.serveTLSClient(conn, br, host)
		return
	}

	// 明文 HTTP：交给 inner server 解析，Handler 用 r.Host 补全目标
	if isHTTPMethod(head) {
		inner := &http.Server{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				host := r.Host
				if host == "" {
					http.Error(w, "missing Host header", http.StatusBadRequest)
					return
				}
				target := &url.URL{Scheme: "http", Host: host, Path: r.URL.Path, RawQuery: r.URL.RawQuery}
				t.proxy.relay(w, r, "http", host, target)
			}),
			ReadHeaderTimeout: 30 * time.Second,
		}
		done := make(chan struct{})
		var ln *singleConnListener
		notify := &notifyCloseConn{Conn: &bufferedConn{Conn: conn, r: br}, onClose: func() { ln.notifyClosed() }}
		ln = &singleConnListener{conn: notify, done: done}
		_ = inner.Serve(ln)
		return
	}

	// 其他协议 → 纯隧道
	t.tunnel(conn, br, tgt)
}

var httpMethods = []string{"GET ", "POST ", "PUT ", "PATCH ", "DELETE ", "HEAD ", "OPTIONS ", "CONNECT ", "TRACE "}

func isHTTPMethod(head []byte) bool {
	for _, m := range httpMethods {
		if len(head) >= len(m) && bytes.EqualFold(head[:len(m)], []byte(m)) {
			return true
		}
	}
	return false
}

// tunnel 未知协议：原样转发到原始目标，记录为 tunnel 会话。
func (t *TransparentServer) tunnel(conn net.Conn, br *bufio.Reader, tgt *redirectTarget) {
	up, err := t.proxy.upstreamDial(context.Background(), "tcp", net.JoinHostPort(tgt.ip, fmt.Sprint(tgt.port)))
	if err != nil {
		return
	}
	defer up.Close()
	sess := &Session{
		ID:        t.proxy.newID(),
		StartedAt: time.Now().UnixMilli(),
		Scheme:    "tunnel",
		Method:    "TCP",
		Host:      net.JoinHostPort(tgt.ip, fmt.Sprint(tgt.port)),
		URL:       "tcp://" + net.JoinHostPort(tgt.ip, fmt.Sprint(tgt.port)),
		Error:     "non-HTTP tunnel (raw relay)",
		ClientAddr: conn.RemoteAddr().String(),
	}
	go func() {
		_, _ = io.Copy(up, br)
		if tc, ok := up.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
	}()
	_, _ = io.Copy(conn, up)
	sess.ElapsedMs = time.Now().UnixMilli() - sess.StartedAt
	t.proxy.store.Add(sess)
}
