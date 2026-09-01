package clientapp

import "strings"

// 连接后非致命告警文案（与 connect_failure / dial_errors 分工）：
//   - FormatConnectFailure：首连 WaitConnected 超时/失败（登录窗、CLI 首连退出）
//   - FormatDialError：拨号/TLS 层拒绝（封禁、源白名单、明文 banner）
//   - MergeConnectWarns：已 Connected 后的部分路由失败 + ICS 异网卡提示（LastError 展示）

// MergeConnectWarns 合并部分路由失败与 ICS 异网卡提示，供 Connected 后 LastError 展示。
func MergeConnectWarns(routeWarn, icsHint string) string {
	routeWarn = strings.TrimSpace(routeWarn)
	icsHint = strings.TrimSpace(icsHint)
	switch {
	case routeWarn == "":
		return icsHint
	case icsHint == "":
		return routeWarn
	default:
		return routeWarn + "\n" + icsHint
	}
}
