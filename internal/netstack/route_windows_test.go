//go:build windows

package netstack

import "testing"

// TestIsWinNATUnavailable 识别 WinNAT 子系统缺失错误码。
func TestIsWinNATUnavailable(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{fmtError("New-NetNat : Invalid class HRESULT 0x80041010"), true},
		{fmtError("New-NetNat : 无效"), true},
		{fmtError("Provider load failure 0x80041013"), true},
		{fmtError("access denied"), false},
	}
	for _, c := range cases {
		if got := isWinNATUnavailable(c.err); got != c.want {
			t.Fatalf("isWinNATUnavailable(%v)=%v want %v", c.err, got, c.want)
		}
	}
}

type fmtError string

func (e fmtError) Error() string { return string(e) }
