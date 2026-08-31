package winnet_test

import (
	"strings"
	"testing"

	"haovpn/internal/brand"
	"haovpn/internal/winnet"
)

// TestPSSnippetAssignAdapterIf 钉死找网卡片段：转义、Wintun 字面量、品牌池、无第二套硬编码 HaoVPN 旁路。
func TestPSSnippetAssignAdapterIf(t *testing.T) {
	ps := winnet.PSSnippetAssignAdapterIf("haovpn0")
	for _, frag := range []string{"haovpn0", "Get-NetAdapter", "Wintun", brand.WintunPool, "InterfaceDescription"} {
		if !strings.Contains(ps, frag) {
			t.Fatalf("missing %q in:\n%s", frag, ps)
		}
	}
	// 单引号须翻倍，避免注入截断
	psQ := winnet.PSSnippetAssignAdapterIf("a'b")
	if !strings.Contains(psQ, "a''b") {
		t.Fatalf("expected escaped quote, got:\n%s", psQ)
	}
}

// TestPSSnippetICSDisableAll 钉死 COM 关共享模板，供 netstack/winnet 共用。
func TestPSSnippetICSDisableAll(t *testing.T) {
	ps := winnet.PSSnippetICSDisableAll()
	for _, frag := range []string{"hnetcfg.dll", "HNetCfg.HNetShare", "DisableSharing", "EnumEveryConnection"} {
		if !strings.Contains(ps, frag) {
			t.Fatalf("missing %q in:\n%s", frag, ps)
		}
	}
	loop := winnet.PSSnippetICSDisableSharingLoop()
	if !strings.Contains(loop, "DisableSharing") {
		t.Fatal("sharing loop missing DisableSharing")
	}
}

// TestPSSnippetICSDisablePair 靶向关共享须含两侧转义名。
func TestPSSnippetICSDisablePair(t *testing.T) {
	ps := winnet.PSSnippetICSDisablePair("WLAN", "haovpn0")
	for _, frag := range []string{"WLAN", "haovpn0", "DisableSharing"} {
		if !strings.Contains(ps, frag) {
			t.Fatalf("missing %q", frag)
		}
	}
}

// TestBuildPrepareWintunOrphanScript 孤儿清理脚本单一真相源（原 tun 私有生成器）。
func TestBuildPrepareWintunOrphanScript(t *testing.T) {
	if winnet.BuildPrepareWintunOrphanScript("") != "" {
		t.Fatal("empty name should yield empty script")
	}
	ps := winnet.BuildPrepareWintunOrphanScript("haovpn0")
	for _, frag := range []string{"haovpn0", "Remove-NetAdapter", brand.WintunPool, "Wintun"} {
		if !strings.Contains(ps, frag) {
			t.Fatalf("missing %q in:\n%s", frag, ps)
		}
	}
}
