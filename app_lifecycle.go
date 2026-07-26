package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	appShutdownMu sync.Mutex
	appShutdownFn context.CancelFunc
	appStartedAt  time.Time
)

func registerAppShutdown(cancel context.CancelFunc) {
	appShutdownMu.Lock()
	appShutdownFn = cancel
	appShutdownMu.Unlock()
}

// resolveExecutablePath 返回真实可执行文件路径（解析 symlink）。
func resolveExecutablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil && resolved != "" {
		exe = resolved
	}
	return filepath.Abs(exe)
}

// chdirToExecutableDir 将进程工作目录切到二进制所在目录，确保 ./config.yaml 等相对路径稳定。
func chdirToExecutableDir() {
	exe, err := resolveExecutablePath()
	if err != nil {
		log.Printf("[系统] 定位程序目录失败: %v", err)
		return
	}
	dir := filepath.Dir(exe)
	if dir == "" {
		return
	}
	if err := os.Chdir(dir); err != nil {
		log.Printf("[系统] 切换工作目录失败: %v", err)
		return
	}
}

func requestAppRestart(reason string) {
	log.Printf("[系统] 请求重启: %s", reason)
	appShutdownMu.Lock()
	cancel := appShutdownFn
	appShutdownMu.Unlock()
	if cancel != nil {
		cancel()
	}
	go func() {
		time.Sleep(2 * time.Second)
		os.Exit(0)
	}()
}
