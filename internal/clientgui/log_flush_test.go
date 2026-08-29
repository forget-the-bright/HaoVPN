package clientgui

import (
	"fmt"
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

// TestAppendLogKeepsLatest300 超过默认展示上限时只保留最近 300 行。
func TestAppendLogKeepsLatest300(t *testing.T) {
	u := &uiApp{logLines: make([]string, 0, 8)}
	total := logDisplayKeep + 50
	for i := 0; i < total; i++ {
		u.appendLog(fmt.Sprintf("line-%d", i))
	}
	n := u.bufferedLogLineCount()
	if n != logDisplayKeep {
		t.Fatalf("应保留 %d 行, got=%d", logDisplayKeep, n)
	}
	text := u.bufferedLogText()
	if strings.Contains(text, "line-0\n") || strings.HasPrefix(text, "line-0") {
		t.Fatal("最早的行应已被裁剪")
	}
	firstKept := fmt.Sprintf("line-%d", total-logDisplayKeep)
	lastKept := fmt.Sprintf("line-%d", total-1)
	if !strings.HasPrefix(text, firstKept) {
		t.Fatalf("首行应为 %q", firstKept)
	}
	if !strings.HasSuffix(text, lastKept) {
		t.Fatalf("末行应为 %q", lastKept)
	}
}
