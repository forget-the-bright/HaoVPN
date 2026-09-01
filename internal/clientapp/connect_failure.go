package clientapp

import (
	"context"
	"strings"

	"haovpn/internal/safeutil"
)

// FormatConnectFailure 将 WaitConnected/Start 失败转为 CLI/GUI 可读文案。
//
// lastError 优先（Engine 已写用户可见句）；仅当失败本身是超时时换成中文提示，
// 避免把真实鉴权/拨号错误盖成「连接超时」。
// 超时判定经 safeutil.IsDeadline（errors.Is），禁止字符串比对 DeadlineExceeded 文案。
func FormatConnectFailure(err error, lastError string, ctxErr error) string {
	msg := strings.TrimSpace(lastError)
	if msg == "" && err != nil {
		msg = err.Error()
	}
	if msg == "" {
		msg = "连接失败"
	}
	deadlineText := context.DeadlineExceeded.Error()
	if safeutil.IsDeadline(err) || msg == deadlineText {
		return "连接超时，请检查服务器地址、网络与密码"
	}
	// WaitConnected 仅因 ctx 截止返回、且尚无更具体 LastError
	if safeutil.IsDeadline(ctxErr) && (strings.TrimSpace(lastError) == "" || lastError == deadlineText) {
		return "连接超时，请检查服务器地址、网络与密码"
	}
	return msg
}
