package main

import (
	"errors"
	"strings"
	"sync"
	"time"
)

// InterceptDecision 是 UI 对一次拦截的裁决：继续（可带修改）或丢弃。
type InterceptDecision struct {
	Action string // "continue" | "drop"

	// 请求侧修改（Action=continue 时可选）
	Method  string
	URL     string
	Headers []Header
	Body    []byte

	// 响应侧修改（Action=continue 时可选）
	Status     int
	StatusText string
}

// Interceptor 管理拦截模式：命中规则的请求/响应在转发前暂停，
// 通过事件通知 UI，等待 UI 裁决（继续/丢弃/修改），超时自动放行原包。
type Interceptor struct {
	mu        sync.Mutex
	enabled   bool
	pattern   string // host 包含匹配，空 = 全部
	direction string // "request" | "response" | "both"
	timeout   time.Duration
	pending   map[string]chan *InterceptDecision
	broadcast func(v any)
}

func NewInterceptor(broadcast func(v any)) *Interceptor {
	return &Interceptor{
		pending:   make(map[string]chan *InterceptDecision),
		broadcast: broadcast,
		timeout:   60 * time.Second,
		direction: "request",
	}
}

func (it *Interceptor) Configure(enabled bool, pattern, direction string, timeout time.Duration) {
	it.mu.Lock()
	defer it.mu.Unlock()
	it.enabled = enabled
	it.pattern = pattern
	if direction == "request" || direction == "response" || direction == "both" {
		it.direction = direction
	}
	if timeout > 0 {
		it.timeout = timeout
	}
}

func (it *Interceptor) Snapshot() (enabled bool, pattern, direction string) {
	it.mu.Lock()
	defer it.mu.Unlock()
	return it.enabled, it.pattern, it.direction
}

func (it *Interceptor) shouldIntercept(dir, host string) bool {
	it.mu.Lock()
	defer it.mu.Unlock()
	if !it.enabled {
		return false
	}
	if it.direction != "both" && it.direction != dir {
		return false
	}
	if it.pattern != "" && !strings.Contains(strings.ToLower(host), strings.ToLower(it.pattern)) {
		return false
	}
	return true
}

// InterceptRequest 发送请求拦截事件并等待 UI 裁决；超时返回 (nil, err) 由调用方放行原包。
func (it *Interceptor) InterceptRequest(sessionID, method, url string, headers []Header, body []byte) (*InterceptDecision, error) {
	ch := it.register(sessionID)
	it.broadcast(map[string]any{
		"type":      "interceptRequest",
		"sessionId": sessionID,
		"request": map[string]any{
			"method":  method,
			"url":     url,
			"headers": headers,
			"body":    body,
		},
	})
	return it.await(sessionID, ch)
}

// InterceptResponse 发送响应拦截事件并等待 UI 裁决。
func (it *Interceptor) InterceptResponse(sessionID string, status int, statusText string, headers []Header, body []byte) (*InterceptDecision, error) {
	ch := it.register(sessionID)
	it.broadcast(map[string]any{
		"type":      "interceptResponse",
		"sessionId": sessionID,
		"response": map[string]any{
			"status":     status,
			"statusText": statusText,
			"headers":    headers,
			"body":       body,
		},
	})
	return it.await(sessionID, ch)
}

func (it *Interceptor) register(id string) chan *InterceptDecision {
	it.mu.Lock()
	defer it.mu.Unlock()
	ch := make(chan *InterceptDecision, 1)
	it.pending[id] = ch
	return ch
}

func (it *Interceptor) await(id string, ch chan *InterceptDecision) (*InterceptDecision, error) {
	select {
	case d := <-ch:
		return d, nil
	case <-time.After(it.timeout):
		it.mu.Lock()
		delete(it.pending, id)
		it.mu.Unlock()
		return nil, errors.New("intercept decision timeout, released as-is")
	}
}

// Resolve 由命令处理器调用，把 UI 裁决投递给正在等待的拦截点。
// 返回 false 表示该拦截已超时或不存在。
func (it *Interceptor) Resolve(id string, d *InterceptDecision) bool {
	it.mu.Lock()
	ch, ok := it.pending[id]
	if ok {
		delete(it.pending, id)
	}
	it.mu.Unlock()
	if !ok {
		return false
	}
	ch <- d
	return true
}
