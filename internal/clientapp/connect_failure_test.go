package clientapp_test

import (
	"context"
	"errors"
	"testing"

	"haovpn/internal/clientapp"
)

func TestFormatConnectFailure(t *testing.T) {
	t.Parallel()
	deadline := context.DeadlineExceeded
	tests := []struct {
		name    string
		err     error
		last    string
		ctxErr  error
		wantSub string
	}{
		{"last_error优先", errors.New("账号或密码错误"), "服务端拒绝", nil, "服务端拒绝"},
		{"err兜底", errors.New("拨号失败"), "", nil, "拨号失败"},
		{"deadline_err", deadline, "", nil, "连接超时"},
		{"deadline_ctx", nil, "", deadline, "连接超时"},
		{"空", nil, "", nil, "连接失败"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := clientapp.FormatConnectFailure(tc.err, tc.last, tc.ctxErr)
			if got == "" {
				t.Fatal("empty")
			}
			if tc.wantSub != "连接超时" && tc.wantSub != "连接失败" && got != tc.wantSub {
				t.Fatalf("got %q want %q", got, tc.wantSub)
			}
			if tc.wantSub == "连接超时" && got != "连接超时，请检查服务器地址、网络与密码" {
				t.Fatalf("got %q", got)
			}
		})
	}
}
