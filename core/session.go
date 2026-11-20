package main

import (
	"sync"
)

// Header 保留原始头名，便于 UI 展示。
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Session 是一个完整的请求/响应对（已解压，体积受 maxBody 限制）。
type Session struct {
	ID         string `json:"id"`
	StartedAt  int64  `json:"startedAt"` // unix 毫秒
	ElapsedMs  int64  `json:"elapsedMs"`
	Scheme     string `json:"scheme"` // http / https / tunnel
	Method     string `json:"method"`
	Host       string `json:"host"`
	URL        string `json:"url"`
	Status     int    `json:"status"`
	StatusText string `json:"statusText"`
	Error      string `json:"error,omitempty"`
	ClientAddr string `json:"clientAddr"`

	RequestHeaders      []Header `json:"requestHeaders"`
	RequestBody         []byte   `json:"requestBody"` // 已解压、已截断
	RequestTruncated    bool     `json:"requestTruncated"`
	RequestContentType  string   `json:"requestContentType"`
	ResponseHeaders     []Header `json:"responseHeaders"`
	ResponseBody        []byte   `json:"responseBody"`
	ResponseTruncated   bool     `json:"responseTruncated"`
	ResponseContentType string   `json:"responseContentType"`
}

// SessionStore 是内存环形缓冲，容量 cap，超出丢弃最旧。
type SessionStore struct {
	mu       sync.RWMutex
	sessions []*Session
	cap      int
	onAdd    func(*Session)
}

func NewSessionStore(cap int) *SessionStore {
	return &SessionStore{cap: cap}
}

// OnAdd 注册新会话回调（事件广播用）。
func (s *SessionStore) OnAdd(fn func(*Session)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onAdd = fn
}

func (s *SessionStore) Add(sess *Session) {
	s.mu.Lock()
	if len(s.sessions) >= s.cap {
		s.sessions = s.sessions[1:]
	}
	s.sessions = append(s.sessions, sess)
	fn := s.onAdd
	s.mu.Unlock()
	if fn != nil {
		fn(sess)
	}
}

func (s *SessionStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions = s.sessions[:0]
}

func (s *SessionStore) Snapshot() []*Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Session, len(s.sessions))
	copy(out, s.sessions)
	return out
}

func (s *SessionStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}
