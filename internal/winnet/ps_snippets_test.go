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

// TestPSSnippetSharedAccessEnsure 默认路径须 Start/already_running，禁止无条件 Restart-Force。
func TestPSSnippetSharedAccessEnsure(t *testing.T) {
	ps := winnet.PSSnippetSharedAccessEnsure()
	for _, frag := range []string{"SharedAccess", "already_running", "Start-Service", "ics_sharedaccess"} {
		if !strings.Contains(ps, frag) {
			t.Fatalf("missing %q in:\n%s", frag, ps)
		}
	}
	if strings.Contains(ps, "Restart-Service") {
		t.Fatal("Ensure 路径不得含 Restart-Service（抖网卡）")
	}
}

// TestPSSnippetSharedAccessRestart 补救路径才允许 Restart-Force。
func TestPSSnippetSharedAccessRestart(t *testing.T) {
	ps := winnet.PSSnippetSharedAccessRestart()
	for _, frag := range []string{"Restart-Service", "action=restart", "SharedAccess"} {
		if !strings.Contains(ps, frag) {
			t.Fatalf("missing %q", frag)
		}
	}
}

// TestPSSnippetPreferVPNAfterICS 钉死嵌入 PreferVPN：轮询 137、恢复 /32、清 TUN 默认路由、SkipAsSource、诊断。
func TestPSSnippetPreferVPNAfterICS(t *testing.T) {
	ps := winnet.PSSnippetPreferVPNAfterICS("10.88.0.2", 23)
	for _, frag := range []string{
		"10.88.0.2", "192.168.137.", "SkipAsSource", "Remove-NetIPAddress",
		"ics_src_diag", "ics_prefer_vpn", "TickCount64", "1500",
		"PrefixLength -ne 32", "ics_prefix_fix", "0.0.0.0/0", "Remove-NetRoute", "ics_default_route_scrubbed",
	} {
		if !strings.Contains(ps, frag) {
			t.Fatalf("missing %q in:\n%s", frag, ps)
		}
	}
}

// TestPSSnippetSkipAsSourceOnly 软换轻量 PreferVPN：仅 SkipAsSource，无 prefix fix / 清默认路由。
func TestPSSnippetSkipAsSourceOnly(t *testing.T) {
	ps := winnet.PSSnippetSkipAsSourceOnly("10.88.0.2", 23)
	for _, frag := range []string{"10.88.0.2", "SkipAsSource", "192.168.137.", "ics_src_diag"} {
		if !strings.Contains(ps, frag) {
			t.Fatalf("missing %q in:\n%s", frag, ps)
		}
	}
	for _, absent := range []string{"PrefixLength -ne 32", "Remove-NetRoute", "Remove-NetIPAddress", "TickCount64"} {
		if strings.Contains(ps, absent) {
			t.Fatalf("轻量片段不应含 %q", absent)
		}
	}
}

// TestPSSnippetICSAlreadyPairedCheck 钉死已配对跳过 Enable 的关键字。
func TestPSSnippetICSAlreadyPairedCheck(t *testing.T) {
	ps := winnet.PSSnippetICSAlreadyPairedCheck()
	for _, frag := range []string{
		"SharingEnabled", "already_paired", "SharingType", "SharingConnectionType", "$script:ok",
	} {
		if !strings.Contains(ps, frag) {
			t.Fatalf("missing %q in:\n%s", frag, ps)
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

// TestPSSnippetNewNetNatEscapesQuote Name/prefix 含单引号须翻倍，禁止裸拼。
func TestPSSnippetNewNetNatEscapesQuote(t *testing.T) {
	name := "hao'nat"
	prefix := "10.0.0.0/24';evil"
	ps := winnet.PSSnippetNewNetNat(name, prefix)
	wantName := winnet.EscapeSingleQuoted(name)
	wantPrefix := winnet.EscapeSingleQuoted(prefix)
	if !strings.Contains(ps, "'"+wantName+"'") {
		t.Fatalf("Name 未正确嵌入: ps=%q", ps)
	}
	if !strings.Contains(ps, "'"+wantPrefix+"'") {
		t.Fatalf("prefix 未正确嵌入: ps=%q", ps)
	}
	if strings.Contains(ps, "-Name 'hao'nat'") {
		t.Fatalf("Name 未转义: %q", ps)
	}
}

// TestPSSnippetRemoveNetNatEscapesQuote Remove-NetNat Name 须 EscapeSingleQuoted。
func TestPSSnippetRemoveNetNatEscapesQuote(t *testing.T) {
	name := "x';y"
	ps := winnet.PSSnippetRemoveNetNat(name)
	want := winnet.EscapeSingleQuoted(name)
	if !strings.Contains(ps, "'"+want+"'") {
		t.Fatalf("Remove-NetNat Name 未转义: ps=%q", ps)
	}
	if strings.Contains(ps, "-Name 'x';y'") {
		t.Fatalf("裸拼单引号: %q", ps)
	}
}

func TestPSSnippetAssignIPv4EscapesQuote(t *testing.T) {
	ps := winnet.PSSnippetAssignIPv4("haovpn0", "10.88.0.2", 32)
	if !strings.Contains(ps, "10.88.0.2") {
		t.Fatalf("missing ip: %s", ps)
	}
	psQ := winnet.PSSnippetAssignIPv4("a'b", "10.88.0.2", 32)
	if !strings.Contains(psQ, "a''b") {
		t.Fatalf("config name not escaped: %s", psQ)
	}
}

func TestPSSnippetProbeICSResidueUsesWildcard(t *testing.T) {
	ps := winnet.PSSnippetProbeICSResidue("haovpn0")
	if !strings.Contains(ps, "192.168.137.*") {
		t.Fatalf("missing ICS wildcard: %s", ps)
	}
}

func TestPSSnippetVerifyInterfaceExistsEscapes(t *testing.T) {
	ps := winnet.PSSnippetVerifyInterfaceExists("eth0")
	if !strings.Contains(ps, "eth0") {
		t.Fatal(ps)
	}
	psQ := winnet.PSSnippetVerifyInterfaceExists("a'b")
	if !strings.Contains(psQ, "a''b") {
		t.Fatalf("not escaped: %s", psQ)
	}
}

func TestPSSnippetScrubDefaultRoute(t *testing.T) {
	ps := winnet.PSSnippetScrubDefaultRoute(42)
	if !strings.Contains(ps, "42") || !strings.Contains(ps, "0.0.0.0/0") {
		t.Fatalf("bad scrub snippet: %s", ps)
	}
}

// TestPSSnippetICSEnableSharing 钉死 ICS EnableSharing 主脚本关键片段与转义。
func TestPSSnippetICSEnableSharing(t *testing.T) {
	ps := winnet.PSSnippetICSEnableSharing(
		"Ethernet", "haovpn0", 23, "HaoVPN",
		winnet.PSSnippetICSDisableSharingLoop(),
		winnet.PSSnippetICSAlreadyPairedCheck(),
		winnet.PSSnippetPreferVPNAfterICS("10.88.0.2", 23),
	)
	for _, frag := range []string{
		"EnableSharing", "ics_enable_ok", "HNetCfg.HNetShare", "Ethernet", "haovpn0",
		"SharedAccess", "already_paired", "10.88.0.2",
	} {
		if !strings.Contains(ps, frag) {
			t.Fatalf("missing %q in:\n%s", frag, ps)
		}
	}
	psQ := winnet.PSSnippetICSEnableSharing("eth'0", "tun'1", 1, "a'b", "", "", "")
	if !strings.Contains(psQ, "eth''0") || !strings.Contains(psQ, "tun''1") || !strings.Contains(psQ, "a''b") {
		t.Fatalf("names not escaped: %s", psQ)
	}
}

func TestPSSnippetFindInterfaceInCIDR(t *testing.T) {
	ps := winnet.PSSnippetFindInterfaceInCIDR("192.168.3.0", "255.255.255.0", "HaoVPN|Wintun")
	for _, frag := range []string{"192.168.3.0", "255.255.255.0", "HaoVPN|Wintun", "Get-NetIPAddress"} {
		if !strings.Contains(ps, frag) {
			t.Fatalf("missing %q: %s", frag, ps)
		}
	}
	psQ := winnet.PSSnippetFindInterfaceInCIDR("10.0.0.0", "255.0.0.0", "a'b")
	if !strings.Contains(psQ, "a''b") {
		t.Fatalf("skip pattern not escaped: %s", psQ)
	}
}

func TestPSSnippetFindInterfaceByRoute(t *testing.T) {
	ps := winnet.PSSnippetFindInterfaceByRoute("192.168.3.254", "HaoVPN|Wintun")
	for _, frag := range []string{"192.168.3.254", "Find-NetRoute", "0.0.0.0/0", "HaoVPN|Wintun"} {
		if !strings.Contains(ps, frag) {
			t.Fatalf("missing %q: %s", frag, ps)
		}
	}
}

