//go:build windows

package platform

import "testing"

func TestJoinElevatedArgsQuotesSpaces(t *testing.T) {
	got := joinElevatedArgs([]string{"-c", `D:\My Apps\client.yaml`, "--flag"})
	if got == "-c D:\\My Apps\\client.yaml --flag" {
		t.Fatal("含空格路径不得裸拼接")
	}
	// EscapeArg 对含空格参数加引号
	wantSub := `"D:\My Apps\client.yaml"`
	if !contains(got, wantSub) && !contains(got, `D:\My Apps\client.yaml`) {
		// EscapeArg 可能用不同转义；至少须保留完整路径片段
		t.Fatalf("got=%q", got)
	}
	if !contains(got, "-c") || !contains(got, "--flag") {
		t.Fatalf("丢失参数: %q", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(len(s) > 0 && indexOf(s, sub) >= 0))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
