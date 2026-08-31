package clientgui

import (
	"strings"
	"testing"
	"time"

	"haovpn/internal/brand"
	"haovpn/internal/clientapp"
)

func TestFormatTrayTooltipConnectedIPFirst(t *testing.T) {
	since := time.Date(2026, 8, 31, 17, 44, 0, 0, time.Local)
	got := formatTrayTooltip(trayTooltipInput{
		State:  clientapp.StateConnected,
		Server: "local.example:8442",
		VPNIP:  "10.88.0.2",
		Since:  since,
	})
	if !strings.Contains(got, "分配 IP: 10.88.0.2") {
		t.Fatalf("missing IP: %q", got)
	}
	if !strings.HasPrefix(got, brand.Name+"\n分配 IP:") {
		t.Fatalf("IP should be second line: %q", got)
	}
	if utf16Len(got) > windowsTrayTipMaxUTF16 {
		t.Fatalf("too long %d tip=%q", utf16Len(got), got)
	}
}

// TestFormatTrayTooltipRealHostNoSinceStub 回归：旧 127 预算被系统砍成「连接自: 20」。
func TestFormatTrayTooltipRealHostNoSinceStub(t *testing.T) {
	since := time.Date(2026, 8, 31, 19, 51, 0, 0, time.Local)
	got := formatTrayTooltip(trayTooltipInput{
		State:  clientapp.StateConnected,
		Server: "local.wuanwanghao.top:8442",
		VPNIP:  "10.88.0.2",
		Since:  since,
	})
	if !strings.Contains(got, "10.88.0.2") {
		t.Fatalf("missing IP: %q", got)
	}
	if strings.Contains(got, "连接自: 20") && !strings.Contains(got, "8/31") {
		t.Fatalf("truncated since stub: %q", got)
	}
	if strings.Contains(got, "连接自:") {
		if !strings.Contains(got, "8/31") || !strings.Contains(got, "19:51") {
			t.Fatalf("since must be complete short date: %q", got)
		}
	}
	if utf16Len(got) > windowsTrayTipMaxUTF16 {
		t.Fatalf("len=%d tip=%q", utf16Len(got), got)
	}
}

func TestFormatTrayTooltipLongHostKeepsIP(t *testing.T) {
	longHost := strings.Repeat("a", 80) + ".example.com:8442"
	got := formatTrayTooltip(trayTooltipInput{
		State:  clientapp.StateConnected,
		Server: longHost,
		VPNIP:  "10.88.0.2",
		Since:  time.Date(2026, 8, 31, 19, 31, 0, 0, time.Local),
	})
	if !strings.Contains(got, "10.88.0.2") {
		t.Fatalf("IP must survive long host: %q", got)
	}
	if utf16Len(got) > windowsTrayTipMaxUTF16 {
		t.Fatalf("len=%d tip=%q", utf16Len(got), got)
	}
}

func TestFormatTrayTooltipDisconnecting(t *testing.T) {
	got := formatTrayTooltip(trayTooltipInput{Phase: trayTipDisconnecting, State: clientapp.StateConnected})
	if !strings.Contains(got, "正在断开") {
		t.Fatalf("got %q", got)
	}
	if strings.Contains(got, "正在连接") {
		t.Fatal("must not say connecting while disconnecting")
	}
}

func TestFormatTrayTooltipConnectingWithIP(t *testing.T) {
	got := formatTrayTooltip(trayTooltipInput{
		State: clientapp.StateConnecting,
		VPNIP: "10.88.0.2",
	})
	if !strings.Contains(got, "配置网络") || !strings.Contains(got, "10.88.0.2") {
		t.Fatalf("got %q", got)
	}
	if utf16Len(got) > windowsTrayTipMaxUTF16 {
		t.Fatalf("too long %d", utf16Len(got))
	}
}

func TestFormatTrayTooltipIdle(t *testing.T) {
	if got := formatTrayTooltip(trayTooltipInput{State: clientapp.StateIdle}); got != brand.Name+"\n未连接" {
		t.Fatalf("%q", got)
	}
}
