package clientapp

import (
	"strings"
)

// IsFatalHandshakeError 判断握手/鉴权失败是否应停止自动重连。
//
// 密码错误、账号已在线、禁用、须改密等不会因重试自行恢复，继续重连只会刷日志并争抢。
// 网络超时、拨号失败等非致命错误仍由 ReconnectClient 退避重试。
func IsFatalHandshakeError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	fatalSubstrs := []string{
		"用户名或密码错误",
		"该账号已在其他设备在线",
		"账号已禁用",
		"须先修改密码",
		"须修改密码",
		"账号未开通 VPN",
		"请提供密码",
		"缺少账号密码",
		"请使用账号密码登录",
		"无效握手请求",
		"登录已锁定",
		"IP 已锁定",
	}
	for _, s := range fatalSubstrs {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}
