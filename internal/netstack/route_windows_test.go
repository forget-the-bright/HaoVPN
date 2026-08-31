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

// TestWinNATUnavailableCache 会话缓存：标记后 isWinNATCachedUnavailable 为 true。
func TestWinNATUnavailableCache(t *testing.T) {
	resetWinNATCacheForTest()
	defer resetWinNATCacheForTest()
	if isWinNATCachedUnavailable() {
		t.Fatal("初始不应缓存")
	}
	markWinNATUnavailable()
	if !isWinNATCachedUnavailable() {
		t.Fatal("标记后应缓存")
	}
}

type fmtError string

func (e fmtError) Error() string { return string(e) }

func TestParseWinNATMatchOutput(t *testing.T) {
	cases := []struct {
		out  string
		want bool
	}{
		{"MATCH", true},
		{"match\r\n", true},
		{"MISS", false},
		{"DIFF", false},
		{"", false},
	}
	for _, c := range cases {
		if got := parseWinNATMatchOutput(c.out); got != c.want {
			t.Fatalf("parseWinNATMatchOutput(%q)=%v want %v", c.out, got, c.want)
		}
	}
}
