package winnet_test

import (
	"runtime"
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
