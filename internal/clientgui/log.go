package clientgui

import (
	"strings"

	"fyne.io/fyne/v2"
)

// appendLog 追加一行日志并在主窗口日志区刷新显示（超 500 行时裁剪保留最近 400 行）。
//
// 登录阶段 logEntry 尚未创建时仅写入 logLines 缓冲；showMain 时须调用 flushLogView。
func (u *uiApp) appendLog(line string) {
	u.logMu.Lock()
	u.logLines = append(u.logLines, line)
	if len(u.logLines) > 500 {
		u.logLines = u.logLines[len(u.logLines)-400:]
	}
	text := strings.Join(u.logLines, "\n")
	n := len(u.logLines)
	u.logMu.Unlock()
	if u.logEntry == nil {
		return
	}
	fyne.Do(func() {
		u.applyLogText(text, n)
	})
}

// flushLogView 将 logLines 缓冲刷入日志控件（showMain 创建 logEntry 后调用）。
//
// 须在 UI 线程或经 fyne.Do 调用；showMain 本身在 UI 线程，可直接调用。
func (u *uiApp) flushLogView() {
	u.logMu.Lock()
	text := strings.Join(u.logLines, "\n")
	n := len(u.logLines)
	u.logMu.Unlock()
	if u.logEntry == nil {
		return
	}
	u.applyLogText(text, n)
}

// applyLogText 写入日志控件文本（调用方保证在 UI 线程且 logEntry 非 nil）。
func (u *uiApp) applyLogText(text string, cursorRow int) {
	u.logSyncing = true
	u.logEntry.SetText(text)
	u.logEntry.CursorRow = cursorRow
	u.logSyncing = false
}

// bufferedLogText 返回当前缓冲日志文本（测试用）。
func (u *uiApp) bufferedLogText() string {
	u.logMu.Lock()
	defer u.logMu.Unlock()
	return strings.Join(u.logLines, "\n")
}
