package clientgui

import (
	"strings"

	"fyne.io/fyne/v2"
)

// appendLog 追加一行日志并在主窗口日志区刷新显示（超 500 行时裁剪保留最近 400 行）。
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
		u.logSyncing = true
		u.logEntry.SetText(text)
		u.logEntry.CursorRow = n
		u.logSyncing = false
	})
}
