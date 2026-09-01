package winnet_test

import (
	"testing"

	"haovpn/internal/winnet"
)

// TestLogICSPowerShellLines 钉死：restart 识别；忽略 ics_prefix_fix；空输出不 panic。
func TestLogICSPowerShellLines(t *testing.T) {
	out := []byte("ics_stage stage=restart ms=1\nics_sharedaccess action=restart\nics_prefix_fix old=24\nics_src_diag ip=10.88.0.2\n")
	info := winnet.LogICSPowerShellLines(out)
	if !info.SawSharedAccessRestart {
		t.Fatal("应识别 action=restart")
	}
	info2 := winnet.LogICSPowerShellLines(nil)
	if info2.SawSharedAccessRestart {
		t.Fatal("空输出不应 saw restart")
	}
}
