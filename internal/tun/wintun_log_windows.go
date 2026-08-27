//go:build windows

package tun

import (
	"io"
	"log"
	"strings"
	"sync"

	"haovpn/internal/logger"
)

var (
	wintunLogOnce sync.Once
	wintunLogPrev io.Writer
)

// installWintunLogger 将 Wintun DLL 日志接入 HaoVPN logger，并过滤预期噪声。
//
// 须在首次调用 wintun.OpenAdapter/CreateAdapter 之前执行（DLL 加载时注册回调）。
// 副作用：替换 log.Default 输出；进程内仅安装一次。
func installWintunLogger() {
	wintunLogOnce.Do(func() {
		wintunLogPrev = log.Default().Writer()
		log.SetOutput(&wintunLogWriter{prev: wintunLogPrev})
		log.SetFlags(0)
	})
}

type wintunLogWriter struct {
	prev io.Writer
}

func (w *wintunLogWriter) Write(p []byte) (int, error) {
	msg := strings.TrimSpace(string(p))
	if msg == "" {
		return len(p), nil
	}
	classifyWintunLog(msg)
	return len(p), nil
}

// classifyWintunLog 按 Wintun DLL 消息内容映射到 HaoVPN 日志级别。
//
// 预期路径（Open 失败后将 Create、驱动已存在等）降为 Debug，避免每次重启误报 ERROR。
func classifyWintunLog(msg string) {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "failed to find matching adapter"),
		strings.Contains(msg, "找不到元素"),
		strings.Contains(lower, "creating adapter"),
		strings.Contains(lower, "using existing driver"),
		strings.Contains(lower, "removed orphaned adapter"):
		logger.Debug("wintun: %s", msg)
	case strings.Contains(lower, "error"), strings.Contains(lower, "fail"):
		logger.Warn("wintun: %s", msg)
	default:
		logger.Info("wintun: %s", msg)
	}
}

// isExpectedWintunDebug 判断 Wintun 消息是否属于预期 Open/Create 路径（供单测）。
func isExpectedWintunDebug(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "failed to find matching adapter") ||
		strings.Contains(msg, "找不到元素") ||
		strings.Contains(lower, "creating adapter") ||
		strings.Contains(lower, "using existing driver") ||
		strings.Contains(lower, "removed orphaned adapter")
}
