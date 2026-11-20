//go:build !windows

package main

import "errors"

// 非 Windows 占位：WinDivert 全流量抓取仅支持 Windows。
type RedirectEngine struct{}

type redirectTarget struct {
	ip   string
	port uint16
}

func NewRedirectEngine() *RedirectEngine { return &RedirectEngine{} }

func (e *RedirectEngine) Start(port int) error {
	return errors.New("WinDivert 全流量抓取仅支持 Windows")
}

func (e *RedirectEngine) Stop() {}

func (e *RedirectEngine) Running() bool { return false }

func (e *RedirectEngine) Err() error { return nil }

func (e *RedirectEngine) RegisterProxyConn(srcIP string, srcPort uint16, dstIP string, dstPort uint16) func() {
	return func() {}
}

func (e *RedirectEngine) lookupRedirect(clientIP string, clientPort uint16) *redirectTarget {
	return nil
}

func (e *RedirectEngine) DeleteRedirect(clientIP string, clientPort uint16) {}

// TransparentServer 占位。
type TransparentServer struct{}

func NewTransparentServer(proxy *Proxy, engine *RedirectEngine) *TransparentServer {
	return &TransparentServer{}
}

func (t *TransparentServer) Start(port int) error {
	return errors.New("WinDivert 全流量抓取仅支持 Windows")
}

func (t *TransparentServer) Stop() {}

func (t *TransparentServer) Running() bool { return false }
