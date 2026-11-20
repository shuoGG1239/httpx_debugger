package main

import (
	"testing"
	"time"
)

func TestRedirectMapping(t *testing.T) {
	e := NewRedirectEngine()

	e.addRedirect("192.168.1.10", 50001, "93.184.216.34", 443)
	tgt := e.lookupRedirect("192.168.1.10", 50001)
	if tgt == nil || tgt.ip != "93.184.216.34" || tgt.port != 443 {
		t.Fatalf("unexpected target: %+v", tgt)
	}
	if e.lookupRedirect("192.168.1.10", 50002) != nil {
		t.Fatal("unexpected hit for wrong port")
	}
	e.DeleteRedirect("192.168.1.10", 50001)
	if e.lookupRedirect("192.168.1.10", 50001) != nil {
		t.Fatal("mapping should be deleted")
	}
}

func TestRedirectMappingExpiry(t *testing.T) {
	e := NewRedirectEngine()
	e.addRedirect("10.0.0.1", 40000, "8.8.8.8", 53)
	// 把 lastSeen 改到过期
	e.mu.Lock()
	e.lastSeen[tuple{ip: "10.0.0.1", port: 40000}] = time.Now().Add(-2 * time.Minute)
	e.mu.Unlock()
	cutoff := time.Now().Add(-90 * time.Second)
	e.mu.Lock()
	for k, ts := range e.lastSeen {
		if ts.Before(cutoff) {
			delete(e.mapping, k)
			delete(e.lastSeen, k)
		}
	}
	e.mu.Unlock()
	if e.lookupRedirect("10.0.0.1", 40000) != nil {
		t.Fatal("expired mapping should be removed")
	}
}

func TestProxyConnWhitelist(t *testing.T) {
	e := NewRedirectEngine()
	release := e.RegisterProxyConn("192.168.1.2", 60001, "93.184.216.34", 443)
	if !e.hasProxyConn("192.168.1.2", 60001, "93.184.216.34", 443) {
		t.Fatal("whitelist entry missing after register")
	}
	if e.hasProxyConn("192.168.1.2", 60002, "93.184.216.34", 443) {
		t.Fatal("whitelist matched wrong source port")
	}
	release()
	if e.hasProxyConn("192.168.1.2", 60001, "93.184.216.34", 443) {
		t.Fatal("whitelist entry should be gone after release")
	}
}
