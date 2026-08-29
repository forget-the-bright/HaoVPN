package clientgui

import (
	"strings"

	"fyne.io/fyne/v2"
)

// logDisplayKeep 主窗口日志区默认最多展示的最近行数。
// 磁盘上的 client.log / client.live.log 不受此限制（仍由 logger 滚动策略管理）。
const logDisplayKeep = 300

// appendLog 追加一行日志并在主窗口日志区刷新显示。
//
// 超过 logDisplayKeep 时裁剪，仅保留最近 logDisplayKeep 行，避免 UI 文本过长卡顿。
// 登录阶段 logEntry 尚未创建时仅写入 logLines 缓冲；showMain 时须调用 flushLogView。
func (u *uiApp) appendLog(line string) {
	u.logMu.Lock()
	u.logLines = append(u.logLines, line)
	if len(u.logLines) > logDisplayKeep {
		u.logLines = u.logLines[len(u.logLines)-logDisplayKeep:]
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

// bufferedLogLineCount 返回当前缓冲行数（测试用）。
func (u *uiApp) bufferedLogLineCount() int {
	u.logMu.Lock()
	defer u.logMu.Unlock()
	return len(u.logLines)
}
