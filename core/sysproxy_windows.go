//go:build windows

package main

import (
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

// internetSettingsKey 是 WinINET 的系统代理注册表位置。
const internetSettingsKey = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

// origProxy 保存接管系统代理前的原始值，停止捕获时原样还原。
var origProxy struct {
	read     bool
	enable   uint32
	server   string
	override string
}

// SetSystemProxy 把本机系统代理指向 127.0.0.1:port（用户级，免管理员），
// 首次接管时先快照原始设置。
func SetSystemProxy(port int) error {
	snapshotOriginalProxy()
	k, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.SetDWordValue("ProxyEnable", 1); err != nil {
		return err
	}
	if err := k.SetStringValue("ProxyServer", fmt.Sprintf("127.0.0.1:%d", port)); err != nil {
		return err
	}
	if err := k.SetStringValue("ProxyOverride", "<local>"); err != nil {
		return err
	}
	notifyWinINet()
	return nil
}

// UnsetSystemProxy 关闭系统代理（仅 ProxyEnable=0，保留 ProxyServer 值）。
// 供手动命令与旧版行为使用；stop 自动恢复走 RestoreSystemProxy。
func UnsetSystemProxy() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.SetDWordValue("ProxyEnable", 0); err != nil {
		return err
	}
	notifyWinINet()
	return nil
}

// RestoreSystemProxy 把系统代理还原为接管前的原始状态（含原有的代理配置）。
func RestoreSystemProxy() error {
	if !origProxy.read {
		return nil
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.SetDWordValue("ProxyEnable", origProxy.enable); err != nil {
		return err
	}
	if err := k.SetStringValue("ProxyServer", origProxy.server); err != nil {
		return err
	}
	if err := k.SetStringValue("ProxyOverride", origProxy.override); err != nil {
		return err
	}
	notifyWinINet()
	origProxy.read = false
	return nil
}

// snapshotOriginalProxy 首次调用时读取当前系统代理设置。
func snapshotOriginalProxy() {
	if origProxy.read {
		return
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.QUERY_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	if v, _, err := k.GetIntegerValue("ProxyEnable"); err == nil {
		origProxy.enable = uint32(v)
	}
	if s, _, err := k.GetStringValue("ProxyServer"); err == nil {
		origProxy.server = s
	}
	if s, _, err := k.GetStringValue("ProxyOverride"); err == nil {
		origProxy.override = s
	}
	origProxy.read = true
}

// notifyWinINet 通知已打开的程序刷新代理设置。
func notifyWinINet() {
	const (
		INTERNET_OPTION_SETTINGS_CHANGED = 39
		INTERNET_OPTION_REFRESH          = 37
	)
	dll := syscall.NewLazyDLL("wininet.dll")
	proc := dll.NewProc("InternetSetOptionW")
	_, _, _ = proc.Call(0, INTERNET_OPTION_SETTINGS_CHANGED, 0, 0)
	_, _, _ = proc.Call(0, INTERNET_OPTION_REFRESH, 0, 0)
}

// CAInstalled 按当前 CA 文件的指纹检测是否已装入当前用户 ROOT 库。
// 按指纹而非证书名，避免同名旧证书残留导致的误判。
func CAInstalled(certPath string) bool {
	fp := caFingerprint(certPath)
	if fp == "" {
		return false
	}
	out, err := exec.Command("certutil", "-store", "-user", "Root", fp).CombinedOutput()
	if err != nil {
		return false
	}
	_ = out
	return true
}

// InstallCA 把根证书装进当前用户 ROOT 库（无需管理员）。
// 若库中已有同名但不同指纹的旧证书（certutil 会拒绝重复安装），先删除再安装。
func InstallCA(certPath string) error {
	fp := caFingerprint(certPath)
	if fp == "" {
		return errors.New("无法解析 CA 证书")
	}
	if CAInstalled(certPath) {
		return nil // 已装同指纹，幂等
	}
	// 清理同名的旧证书残留（本工具专属 CN，安全）
	_ = exec.Command("certutil", "-user", "-delstore", "Root", caCN).Run()
	cmd := exec.Command("certutil", "-user", "-addstore", "Root", certPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("certutil addstore failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// caFingerprint 计算 CA 证书文件（PEM）的 SHA1 指纹（大写十六进制，无分隔）。
func caFingerprint(certPath string) string {
	pemBytes, err := os.ReadFile(certPath)
	if err != nil {
		return ""
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return ""
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return ""
	}
	sum := sha1.Sum(cert.Raw)
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}
