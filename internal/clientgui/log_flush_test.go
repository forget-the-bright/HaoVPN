package clientgui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// TestFlushLogViewAfterBufferedAppend 登录阶段缓冲的日志在 flush 后应出现在控件中。
func TestFlushLogViewAfterBufferedAppend(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	u := &uiApp{logLines: make([]string, 0, 8)}
	u.appendLog("line-auth-ok")
	u.appendLog("line-tun")
	if u.logEntry != nil {
		t.Fatal("缓冲阶段不应创建 logEntry")
	}
	if !strings.Contains(u.bufferedLogText(), "line-auth-ok") {
		t.Fatal("缓冲应含鉴权日志")
	}

	u.logEntry = widget.NewMultiLineEntry()
	u.flushLogView()
	got := u.logEntry.Text
	if !strings.Contains(got, "line-auth-ok") || !strings.Contains(got, "line-tun") {
		t.Fatalf("flush 后日志框应含缓冲内容, got=%q", got)
	}
}
