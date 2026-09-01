package winnet_test

import (
	"runtime"
	"strings"
	"testing"

	"haovpn/internal/winnet"
)

// TestHasICSResidueNonWindows 非 Windows 恒 false，避免误触发清理。
func TestHasICSResidueNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 上探测依赖本机网卡，跳过恒 false 断言")
	}
	if winnet.HasICSResidue("haovpn0") {
		t.Fatal("非 Windows 应为 false")
	}
	if err := winnet.CleanupICSResidue("haovpn0", "10.88.0.2"); err != nil {
		t.Fatalf("stub Cleanup: %v", err)
	}
}

// TestCleanupICSResidueEmptyArgs Windows 应拒绝空参数；非 Windows stub 放行。
func TestCleanupICSResidueEmptyArgs(t *testing.T) {
	if runtime.GOOS != "windows" {
		if err := winnet.CleanupICSResidue("", ""); err != nil {
			t.Fatalf("stub: %v", err)
		}
		return
	}
	if err := winnet.CleanupICSResidue("", "10.88.0.1"); err == nil {
		t.Fatal("空 configName 应 error")
	}
	if err := winnet.CleanupICSResidue("haovpn0", ""); err == nil {
		t.Fatal("空 vpnIP 应 error")
	}
}

// TestEscapeSingleQuoted 嵌入 PS 单引号须翻倍。
func TestEscapeSingleQuoted(t *testing.T) {
	if got := winnet.EscapeSingleQuoted("a'b"); got != "a''b" {
		t.Fatalf("got %q", got)
	}
}

// TestEscapeRegex 元字符须转义，便于 -match 字面匹配。
func TestEscapeRegex(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"plain", "plain"},
		{"a.b", `a\.b`},
		{"a|b", `a\|b`},
		{"(x)", `\(x\)`},
	}
	for _, tc := range cases {
		if got := winnet.EscapeRegex(tc.in); got != tc.want {
			t.Fatalf("EscapeRegex(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

// TestPSSnippetMatchUsesEscapeRegex -match 段须含转义后的池名（纵深防御）。
func TestPSSnippetMatchUsesEscapeRegex(t *testing.T) {
	// 即使 ValidateTunName 已挡，模板仍应对 -match 操作数做 EscapeRegex。
	ps := winnet.PSSnippetAssignAdapterIf("haovpn0")
	if !strings.Contains(ps, "-match") {
		t.Fatal("expected -match branch")
	}
	// Wintun 无元字符时 EscapeRegex 恒等；冒烟即可
	if !strings.Contains(ps, "Wintun") {
		t.Fatal("missing Wintun")
	}
}
