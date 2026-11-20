//go:build !windows

package main

import "errors"

// 非 Windows 平台占位实现（本工具面向 Windows，仅保证可编译）。

func SetSystemProxy(port int) error {
	return errors.New("system proxy is only supported on Windows")
}

func UnsetSystemProxy() error {
	return errors.New("system proxy is only supported on Windows")
}

func RestoreSystemProxy() error {
	return errors.New("system proxy is only supported on Windows")
}

func CAInstalled(certPath string) bool { return false }

func InstallCA(certPath string) error {
	return errors.New("CA install is only supported on Windows")
}
