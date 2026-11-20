//go:build windows

package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// startTransparent 起一个不依赖驱动（不 Start 引擎）的透明端口测试环境。
func startTransparent(t *testing.T) (*TransparentServer, *RedirectEngine, *httptest.Server, func()) {
	t.Helper()
	ca, err := LoadOrCreateCA(t.TempDir())
	if err != nil {
		t.Fatalf("ca: %v", err)
	}
	store := NewSessionStore(1000)
	engine := NewRedirectEngine()
	proxy := NewProxy(ca, store, 1<<20, nil, engine)
	ts := NewTransparentServer(proxy, engine)
	if err := ts.Start(8897); err != nil {
		t.Fatalf("transparent start: %v", err)
	}
	echo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fmt.Fprintf(w, "echo:%s:%s:%s", r.Method, r.URL.Path, string(body))
	}))
	cleanup := func() {
		ts.Stop()
		echo.Close()
	}
	return ts, engine, echo, cleanup
}

// dialTransparent 模拟 WinDivert 重定向：建立一对连接（客户端 + 服务端视角），
// 并按服务端视角的客户端地址预置映射，返回 (clientConn, serverConn)。
func dialTransparent(t *testing.T, engine *RedirectEngine, dstPort int) (net.Conn, net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	server, err := ln.Accept()
	ln.Close()
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	saddr := server.RemoteAddr().(*net.TCPAddr)
	engine.addRedirect(saddr.IP.String(), uint16(saddr.Port), "127.0.0.1", uint16(dstPort))
	return client, server
}

func TestTransparentPlainHTTP(t *testing.T) {
	ts, engine, echo, cleanup := startTransparent(t)
	defer cleanup()
	_, portStr, _ := net.SplitHostPort(echo.Listener.Addr().String())
	var echoPort int
	fmt.Sscanf(portStr, "%d", &echoPort)

	conn, server := dialTransparent(t, engine, echoPort)
	defer conn.Close()
	go ts.handleConn(server)

	fmt.Fprintf(conn, "POST /hello HTTP/1.1\r\nHost: 127.0.0.1:%d\r\nContent-Type: text/plain\r\nConnection: close\r\nContent-Length: 5\r\n\r\nworld", echoPort)
	br := bufio.NewReader(conn)
	line, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if !strings.Contains(line, "200") {
		t.Fatalf("status line = %q, want 200", line)
	}
	// 读响应头，按 Content-Length 读 body
	var contentLength int
	for {
		hl, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("read header: %v", err)
		}
		if hl == "\r\n" {
			break
		}
		if name, val, ok := strings.Cut(hl, ":"); ok && strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			fmt.Sscanf(strings.TrimSpace(val), "%d", &contentLength)
		}
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(br, body); err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "echo:POST:/hello:world" {
		t.Fatalf("body = %q", string(body))
	}
	// 会话记录断言
	time.Sleep(200 * time.Millisecond)
	if ts.proxy.store.Len() == 0 {
		t.Fatal("no session recorded")
	}
	last := ts.proxy.store.Snapshot()
	sess := last[len(last)-1]
	if sess.Scheme != "http" || sess.URL != fmt.Sprintf("http://127.0.0.1:%d/hello", echoPort) {
		t.Fatalf("session = %s %s", sess.Scheme, sess.URL)
	}
	if string(sess.RequestBody) != "world" {
		t.Fatalf("request body = %q", sess.RequestBody)
	}
}

func TestTransparentTLSMITM(t *testing.T) {
	ts, engine, _, cleanup := startTransparent(t)
	defer cleanup()

	conn, server := dialTransparent(t, engine, 443)
	defer conn.Close()
	go ts.handleConn(server)

	// 客户端 TLS 握手（不校验服务端证书，但读 peer cert 断言 SAN）
	tlsCfg := &tls.Config{ServerName: "example.com", InsecureSkipVerify: true, NextProtos: []string{"http/1.1"}}
	tlsConn := tls.Client(conn, tlsCfg)
	if err := tlsConn.Handshake(); err != nil {
		t.Fatalf("tls handshake: %v", err)
	}
	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		t.Fatal("no peer cert")
	}
	leaf := state.PeerCertificates[0]
	if len(leaf.DNSNames) == 0 || leaf.DNSNames[0] != "example.com" {
		t.Fatalf("leaf SAN = %v, want example.com", leaf.DNSNames)
	}
	// 发一个 HTTPS 请求：上游 example.com 不可达 → 应收到 502（证明 SNI→MITM→relay 链路通了）
	fmt.Fprintf(tlsConn, "GET /x HTTP/1.1\r\nHost: example.com\r\n\r\n")
	br := bufio.NewReader(tlsConn)
	line, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if !strings.Contains(line, "HTTP/1.1") {
		t.Fatalf("bad status line: %q", line)
	}
	// 完整链路：SNI→MITM→relay→上游→响应回传。上游可达时返回其状态码（如 404），不可达则 502。
	time.Sleep(300 * time.Millisecond)
	last := ts.proxy.store.Snapshot()
	if len(last) == 0 {
		t.Fatal("no session recorded")
	}
	sess := last[len(last)-1]
	if sess.Scheme != "https" || sess.Host != "example.com:443" {
		t.Fatalf("session = %s %s", sess.Scheme, sess.Host)
	}
	if sess.Status == 0 {
		t.Fatal("session status not recorded")
	}
	if len(sess.ResponseBody) == 0 && sess.Error == "" {
		t.Fatal("expected response body or recorded upstream error")
	}
	_ = tlsConn.Close()
}
