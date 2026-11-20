package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"
	"time"
)

// ModifiedRequest 是 UI 裁决请求拦截时带回的（可选修改后的）请求。
type ModifiedRequest struct {
	Method  string   `json:"method,omitempty"`
	URL     string   `json:"url,omitempty"`
	Headers []Header `json:"headers,omitempty"`
	Body    []byte   `json:"body,omitempty"`
}

// ModifiedResponse 是 UI 裁决响应拦截时带回的（可选修改后的）响应。
type ModifiedResponse struct {
	Status     int      `json:"status,omitempty"`
	StatusText string   `json:"statusText,omitempty"`
	Headers    []Header `json:"headers,omitempty"`
	Body       []byte   `json:"body,omitempty"`
}

// Command 是 UI → core 的控制命令（JSON 行协议）。
type Command struct {
	ID      int    `json:"id"`
	Cmd     string `json:"cmd"`
	Listen  string `json:"listen,omitempty"`
	MaxBody int    `json:"maxBody,omitempty"`
	Port    int    `json:"port,omitempty"`

	// setIntercept
	Enabled    *bool  `json:"enabled,omitempty"`
	Pattern    string `json:"pattern,omitempty"`
	Direction  string `json:"direction,omitempty"`
	TimeoutMs  int    `json:"timeoutMs,omitempty"`

	// intercept*/replay
	SessionID string           `json:"sessionId,omitempty"`
	Request   *ModifiedRequest `json:"request,omitempty"`
	Response  *ModifiedResponse `json:"response,omitempty"`
}

// Status 是 core 当前状态快照。
type Status struct {
	Capturing   bool   `json:"capturing"`
	Listen      string `json:"listen"`
	ProxySet    bool   `json:"proxySet"`
	DataDir     string `json:"dataDir"`
	CAInstalled bool   `json:"caInstalled"`
	MaxBody     int    `json:"maxBody"`
	CertPath    string `json:"certPath"`
	KeyPath     string `json:"keyPath"`

	InterceptEnabled   bool   `json:"interceptEnabled"`
	InterceptPattern   string `json:"interceptPattern"`
	InterceptDirection string `json:"interceptDirection"`

	RedirectEnabled bool   `json:"redirectEnabled"`
	RedirectPort    int    `json:"redirectPort"`
	RedirectError   string `json:"redirectError,omitempty"`
}

// EventServer 同时承担：控制命令入口 + 事件广播（会话/状态/拦截），单条 TCP 连接复用。
type EventServer struct {
	proxy       *Proxy
	store       *SessionStore
	interceptor *Interceptor
	transparent *TransparentServer
	dataDir     string
	maxBody     int

	ln      net.Listener
	mu      sync.Mutex
	clients map[net.Conn]struct{}
	status  Status
}

func NewEventServer(proxy *Proxy, store *SessionStore, interceptor *Interceptor, transparent *TransparentServer, dataDir string, maxBody int) *EventServer {
	s := &EventServer{
		proxy:       proxy,
		store:       store,
		interceptor: interceptor,
		transparent: transparent,
		dataDir:     dataDir,
		maxBody:     maxBody,
		clients:     make(map[net.Conn]struct{}),
		status:      Status{DataDir: dataDir, MaxBody: maxBody, CertPath: proxy.ca.CertPath(), KeyPath: proxy.ca.KeyPath()},
	}
	store.OnAdd(func(sess *Session) {
		s.broadcast(map[string]any{"type": "session", "session": sess})
	})
	return s
}

func (s *EventServer) Start(port int) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.ln = ln
	log.Printf("event server listening on %s", addr)
	go s.acceptLoop()
	return nil
}

func (s *EventServer) acceptLoop() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.clients[c] = struct{}{}
		s.mu.Unlock()
		go s.handleClient(c)
	}
}

func (s *EventServer) handleClient(c net.Conn) {
	defer func() {
		s.mu.Lock()
		delete(s.clients, c)
		s.mu.Unlock()
		_ = c.Close()
	}()
	s.send(c, s.currentStatus())
	sc := bufio.NewScanner(c)
	sc.Buffer(make([]byte, 0, 4096), 1<<16)
	for sc.Scan() {
		var cmd Command
		if err := json.Unmarshal(sc.Bytes(), &cmd); err != nil {
			continue
		}
		reply := s.execCommand(cmd)
		s.send(c, reply)
	}
}

// listenPort 从监听地址解析端口（失败回退 8899）。
func listenPort(listen string) int {
	_, portStr, err := net.SplitHostPort(listen)
	if err != nil {
		return 8899
	}
	p, err := strconv.Atoi(portStr)
	if err != nil {
		return 8899
	}
	return p
}

func (s *EventServer) currentStatus() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.status
	st.CAInstalled = CAInstalled(s.proxy.ca.CertPath())
	ie, ip, id := s.interceptor.Snapshot()
	st.InterceptEnabled = ie
	st.InterceptPattern = ip
	st.InterceptDirection = id
	st.RedirectEnabled = s.transparent.Running()
	st.RedirectPort = defaultTransparentPort
	if s.proxy.engine != nil && s.proxy.engine.Err() != nil {
		st.RedirectError = s.proxy.engine.Err().Error()
	}
	return map[string]any{"type": "status", "status": st}
}

func (s *EventServer) execCommand(cmd Command) map[string]any {
	reply := map[string]any{"type": "reply", "id": cmd.ID, "ok": true}
	switch cmd.Cmd {	case "start":
		listen := cmd.Listen
		if listen == "" {
			listen = "127.0.0.1:8899"
		}
		if err := s.proxy.Start(listen); err != nil {
			reply["ok"] = false
			reply["error"] = err.Error()
			return reply
		}
		// 开始捕获 = 自动接管系统代理
		port := listenPort(listen)
		if err := SetSystemProxy(port); err != nil {
			reply["error"] = "代理已监听，但系统代理接管失败: " + err.Error()
		}
		s.mu.Lock()
		s.status.Capturing = true
		s.status.Listen = listen
		s.status.ProxySet = true
		s.mu.Unlock()
		s.broadcast(s.currentStatus())
	case "stop":
		s.proxy.Stop()
		// 停止捕获 = 自动还原系统代理
		if err := RestoreSystemProxy(); err != nil {
			reply["error"] = "代理已停止，但系统代理还原失败: " + err.Error()
		}
		s.mu.Lock()
		s.status.Capturing = false
		s.status.ProxySet = false
		s.mu.Unlock()
		s.broadcast(s.currentStatus())
	case "setProxy":
		port := cmd.Port
		if port == 0 {
			port = 8899
		}
		if err := SetSystemProxy(port); err != nil {
			reply["ok"] = false
			reply["error"] = err.Error()
			return reply
		}
		s.mu.Lock()
		s.status.ProxySet = true
		s.mu.Unlock()
		s.broadcast(s.currentStatus())
	case "unsetProxy":
		if err := UnsetSystemProxy(); err != nil {
			reply["ok"] = false
			reply["error"] = err.Error()
			return reply
		}
		s.mu.Lock()
		s.status.ProxySet = false
		s.mu.Unlock()
		s.broadcast(s.currentStatus())
	case "installCA":
		if err := InstallCA(s.proxy.ca.CertPath()); err != nil {
			reply["ok"] = false
			reply["error"] = err.Error()
			return reply
		}
		s.broadcast(s.currentStatus())
	case "clear":
		s.store.Clear()
	case "setIntercept":
		enabled := false
		if cmd.Enabled != nil {
			enabled = *cmd.Enabled
		}
		s.interceptor.Configure(enabled, cmd.Pattern, cmd.Direction, time.Duration(cmd.TimeoutMs)*time.Millisecond)
		s.broadcast(s.currentStatus())
	case "redirectStart":
		port := cmd.Port
		if port == 0 {
			port = defaultTransparentPort
		}
		if err := s.proxy.engine.Start(port); err != nil {
			reply["ok"] = false
			reply["error"] = err.Error()
			return reply
		}
		if err := s.transparent.Start(port); err != nil {
			s.proxy.engine.Stop()
			reply["ok"] = false
			reply["error"] = err.Error()
			return reply
		}
		s.broadcast(s.currentStatus())
	case "redirectStop":
		s.proxy.engine.Stop()
		s.transparent.Stop()
		s.broadcast(s.currentStatus())
	case "interceptContinue":
		d := &InterceptDecision{Action: "continue"}
		if cmd.Request != nil {
			d.Method = cmd.Request.Method
			d.URL = cmd.Request.URL
			d.Headers = cmd.Request.Headers
			d.Body = cmd.Request.Body
		}
		if !s.interceptor.Resolve(cmd.SessionID, d) {
			reply["ok"] = false
			reply["error"] = "intercept not found or timed out"
		}
	case "interceptResponseContinue":
		d := &InterceptDecision{Action: "continue"}
		if cmd.Response != nil {
			d.Status = cmd.Response.Status
			d.StatusText = cmd.Response.StatusText
			d.Headers = cmd.Response.Headers
			d.Body = cmd.Response.Body
		}
		if !s.interceptor.Resolve(cmd.SessionID, d) {
			reply["ok"] = false
			reply["error"] = "intercept not found or timed out"
		}
	case "interceptDrop":
		if !s.interceptor.Resolve(cmd.SessionID, &InterceptDecision{Action: "drop"}) {
			reply["ok"] = false
			reply["error"] = "intercept not found or timed out"
		}
	case "replay":
		if cmd.Request == nil {
			reply["ok"] = false
			reply["error"] = "replay requires request"
			return reply
		}
		if err := s.proxy.Replay(cmd.Request.Method, cmd.Request.URL, cmd.Request.Headers, cmd.Request.Body); err != nil {
			reply["ok"] = false
			reply["error"] = err.Error()
			return reply
		}
	case "status":
		reply["status"] = s.currentStatus()
	default:
		reply["ok"] = false
		reply["error"] = "unknown command: " + cmd.Cmd
	}
	return reply
}

func (s *EventServer) send(c net.Conn, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, _ = c.Write(append(data, '\n'))
}

func (s *EventServer) broadcast(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.clients {
		_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err := c.Write(append(data, '\n')); err != nil {
			delete(s.clients, c)
			_ = c.Close()
		}
	}
}
