//go:build windows

package main

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/bogushevich/godivert"
	"github.com/bogushevich/godivert/header"
	"golang.org/x/sys/windows"
)

// 透明代理端口：出站 TCP 被重定向到 127.0.0.1:port。
const defaultTransparentPort = 8897

// RedirectEngine 用 WinDivert 实现透明重定向（Proxifier 式）：
//   - handleOut：出站 TCP（目标非回环）全部改目的为 127.0.0.1:8897；SYN 记录「客户端 4-tuple → 原目标」映射
//   - handleBack：从 127.0.0.1:8897 发出的回程包改源为原目标，客户端感知不到代理存在
//   - 代理自身连上游的连接 dial 后注册白名单，避免被再次重定向（环路防护）
type RedirectEngine struct {
	mu      sync.Mutex
	running bool
	port    int
	err     error

	mapping   map[tuple]*redirectTarget // 客户端 (srcIP,srcPort) → 原目标
	lastSeen  map[tuple]time.Time
	whitelist map[proxyConn]struct{} // 代理自身连接

	handleOut  *godivert.WinDivertHandle
	handleBack *godivert.WinDivertHandle
	stopCh     chan struct{}
	doneCh     chan struct{}
	doneOnce   sync.Once
}

type tuple struct {
	ip   string
	port uint16
}

type redirectTarget struct {
	ip   string
	port uint16
}

type proxyConn struct {
	srcIP   string
	srcPort uint16
	dstIP   string
	dstPort uint16
}

func NewRedirectEngine() *RedirectEngine {
	return &RedirectEngine{
		mapping:   make(map[tuple]*redirectTarget),
		lastSeen:  make(map[tuple]time.Time),
		whitelist: make(map[proxyConn]struct{}),
	}
}

// Start 打开 WinDivert 句柄并启动两个收包循环。需要：WinDivert.dll 在 exe 目录，
// 管理员权限（首次自动安装驱动）。
func (e *RedirectEngine) Start(port int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running {
		return nil
	}
	if port == 0 {
		port = defaultTransparentPort
	}

	out, back, err := e.openHandles(port)
	if err != nil {
		e.err = err
		return e.err
	}

	e.port = port
	e.handleOut = out
	e.handleBack = back
	e.stopCh = make(chan struct{})
	e.doneCh = make(chan struct{})
	e.running = true
	e.err = nil
	go e.loopOut()
	go e.loopBack()
	go e.expireLoop()
	return nil
}

// Stop 关闭句柄，两个收包循环随之退出。
func (e *RedirectEngine) Stop() {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return
	}
	e.running = false
	close(e.stopCh)
	out, back := e.handleOut, e.handleBack
	e.mu.Unlock()

	if out != nil {
		out.Close()
	}
	if back != nil {
		back.Close()
	}
	select {
	case <-e.doneCh:
	case <-time.After(3 * time.Second):
	}
}

func (e *RedirectEngine) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

// Err 返回最近一次错误（如驱动加载失败）。
func (e *RedirectEngine) Err() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.err
}

// openHandles 打开两个 WinDivert 句柄。godivert 在 DLL 缺失时内部会 panic，
// 这里用 recover 兜底转为友好错误，避免 core 崩溃。
func (e *RedirectEngine) openHandles(port int) (out, back *godivert.WinDivertHandle, err error) {
	defer func() {
		if r := recover(); r != nil {
			out = nil
			back = nil
			err = fmt.Errorf("WinDivert 加载失败: %v（请先以管理员运行 scripts/setup-windivert.ps1）", r)
		}
	}()

	out, err = godivert.OpenHandle(
		"outbound && tcp && ip.DstAddr != 127.0.0.1",
		godivert.LayerNetwork,
		0,
		godivert.OpenFlagNone,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("WinDivert 出站句柄打开失败: %w", describeWinDivertError(err))
	}
	back, err = godivert.OpenHandle(
		fmt.Sprintf("outbound && tcp && ip.SrcAddr == 127.0.0.1 && tcp.SrcPort == %d", port),
		godivert.LayerNetwork,
		1,
		godivert.OpenFlagNone,
	)
	if err != nil {
		out.Close()
		return nil, nil, fmt.Errorf("WinDivert 回程句柄打开失败: %w", describeWinDivertError(err))
	}
	return out, back, nil
}

// describeWinDivertError 把 WinDivert 常见错误转成可读提示。
func describeWinDivertError(err error) error {
	if err == nil {
		return nil
	}
	var errno windows.Errno
	if errors.As(err, &errno) {
		switch errno {
		case windows.ERROR_ACCESS_DENIED:
			return fmt.Errorf("权限不足：需要以管理员身份运行（驱动安装/加载需要管理员）")
		case windows.ERROR_MOD_NOT_FOUND, windows.ERROR_FILE_NOT_FOUND:
			return fmt.Errorf("未找到 WinDivert.dll：请先运行 scripts/setup-windivert.ps1（管理员）")
		case windows.ERROR_DRIVER_BLOCKED:
			return fmt.Errorf("WinDivert 驱动被系统阻止（可能需要签名或安全策略放行）")
		}
	}
	return err
}

func (e *RedirectEngine) loopOut() {
	defer e.finish()
	for {
		select {
		case <-e.stopCh:
			return
		default:
		}
		pkt, err := e.handleOut.Recv()
		if err != nil {
			if e.isStopped() {
				return
			}
			e.mu.Lock()
			e.err = describeWinDivertError(err)
			e.mu.Unlock()
			return
		}
		e.processOut(pkt)
	}
}

func (e *RedirectEngine) loopBack() {
	defer e.finish()
	for {
		select {
		case <-e.stopCh:
			return
		default:
		}
		pkt, err := e.handleBack.Recv()
		if err != nil {
			if e.isStopped() {
				return
			}
			e.mu.Lock()
			e.err = describeWinDivertError(err)
			e.mu.Unlock()
			return
		}
		e.processBack(pkt)
	}
}

func (e *RedirectEngine) isStopped() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return !e.running
}

func (e *RedirectEngine) finish() {
	e.mu.Lock()
	e.doneOnce.Do(func() { close(e.doneCh) })
	e.mu.Unlock()
}

// processOut 处理出站包：放行代理自身连接与回程包，其余重定向到透明端口。
func (e *RedirectEngine) processOut(pkt *godivert.Packet) {
	pkt.ParseHeaders()
	tcpHdr, ok := pkt.NextHeader.(*header.TCPHeader)
	if !ok {
		_, _ = pkt.Send(e.handleOut)
		return
	}
	srcIP := pkt.SrcIP().String()
	dstIP := pkt.DstIP().String()
	srcPort, _ := tcpHdr.SrcPort()
	dstPort, _ := tcpHdr.DstPort()

	// 1. 代理自身连上游的连接 → 放行（环路防护）
	if e.hasProxyConn(srcIP, srcPort, dstIP, dstPort) {
		_, _ = pkt.Send(e.handleOut)
		return
	}
	// 2. 回程包（目的已是已知客户端 4-tuple，由 handleBack 修正过源）→ 放行
	if e.lookupRedirect(dstIP, dstPort) != nil {
		_, _ = pkt.Send(e.handleOut)
		return
	}
	// 3. 未知连接的非 SYN 数据 → 放行（不破坏既有连接）
	if !(tcpHdr.SYN() && !tcpHdr.ACK()) && e.lookupRedirect(srcIP, srcPort) == nil {
		_, _ = pkt.Send(e.handleOut)
		return
	}
	// 4. 新 SYN：记录原目标映射
	if tcpHdr.SYN() && !tcpHdr.ACK() {
		e.addRedirect(srcIP, srcPort, dstIP, dstPort)
	}
	// 5. 改目的到透明端口
	pkt.SetDstIP(net.ParseIP("127.0.0.1"))
	_ = pkt.SetDstPort(uint16(e.port))
	_, _ = pkt.Send(e.handleOut)
}

// processBack 处理从透明端口发出的回程包：改源为原目标。
func (e *RedirectEngine) processBack(pkt *godivert.Packet) {
	pkt.ParseHeaders()
	tcpHdr, ok := pkt.NextHeader.(*header.TCPHeader)
	if !ok {
		_, _ = pkt.Send(e.handleBack)
		return
	}
	dstIP := pkt.DstIP().String()
	dstPort, _ := tcpHdr.DstPort()

	tgt := e.lookupRedirect(dstIP, dstPort)
	if tgt == nil {
		// 非重定向连接（直连 8897 的程序），原样放行
		_, _ = pkt.Send(e.handleBack)
		return
	}
	pkt.SetSrcIP(net.ParseIP(tgt.ip))
	_ = pkt.SetSrcPort(tgt.port)
	_, _ = pkt.Send(e.handleBack)
}

func (e *RedirectEngine) expireLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-e.stopCh:
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-90 * time.Second)
			e.mu.Lock()
			for k, t := range e.lastSeen {
				if t.Before(cutoff) {
					delete(e.mapping, k)
					delete(e.lastSeen, k)
				}
			}
			e.mu.Unlock()
		}
	}
}

// ---- 映射表与白名单（供透明连接处理器使用）----

func (e *RedirectEngine) addRedirect(clientIP string, clientPort uint16, dstIP string, dstPort uint16) {
	e.mu.Lock()
	defer e.mu.Unlock()
	k := tuple{ip: clientIP, port: clientPort}
	e.mapping[k] = &redirectTarget{ip: dstIP, port: dstPort}
	e.lastSeen[k] = time.Now()
}

func (e *RedirectEngine) lookupRedirect(clientIP string, clientPort uint16) *redirectTarget {
	e.mu.Lock()
	defer e.mu.Unlock()
	k := tuple{ip: clientIP, port: clientPort}
	if t, ok := e.mapping[k]; ok {
		e.lastSeen[k] = time.Now()
		return t
	}
	return nil
}

func (e *RedirectEngine) DeleteRedirect(clientIP string, clientPort uint16) {
	e.mu.Lock()
	defer e.mu.Unlock()
	k := tuple{ip: clientIP, port: clientPort}
	delete(e.mapping, k)
	delete(e.lastSeen, k)
}

func (e *RedirectEngine) RegisterProxyConn(srcIP string, srcPort uint16, dstIP string, dstPort uint16) func() {
	pc := proxyConn{srcIP: srcIP, srcPort: srcPort, dstIP: dstIP, dstPort: dstPort}
	e.mu.Lock()
	e.whitelist[pc] = struct{}{}
	e.mu.Unlock()
	return func() {
		e.mu.Lock()
		delete(e.whitelist, pc)
		e.mu.Unlock()
	}
}

func (e *RedirectEngine) hasProxyConn(srcIP string, srcPort uint16, dstIP string, dstPort uint16) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.whitelist[proxyConn{srcIP: srcIP, srcPort: srcPort, dstIP: dstIP, dstPort: dstPort}]
	return ok
}
