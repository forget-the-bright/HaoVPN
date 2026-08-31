package clientapp

import (
	"haovpn/internal/autherr"
	"haovpn/internal/logger"
)

// accountOnlineMaxRetries 首次登录时「账号已在线」的最大自动重试次数。
// 须覆盖服务端半死会话静默阈值（约 8～20s）+ 退避，避免未等顶替就停。
// 仅 clientapp 拨号路径使用，不与 SCM DefaultServiceStopTimeout 混用。
const accountOnlineMaxRetries = 40

// IsFatalHandshakeError 判断握手/鉴权失败是否应停止自动重连。
//
// 密码错误、禁用、须改密、IP 锁定等不会因重试自行恢复。
// 「账号已在线」由 ShouldFailFastHandshake 结合有限重试计数处理，本函数对其返回 false。
func IsFatalHandshakeError(err error) bool {
	if err == nil {
		return false
	}
	if autherr.IsIPBanned(err) {
		return true
	}
	if autherr.IsAccountAlreadyOnline(err) {
		return false
	}
	return autherr.IsFatalAuth(err)
}

// ShouldFailFastHandshake 结合 Engine 有限重试，决定是否因本次握手失败停止重连。
//
// 「账号已在线」判定直接走 autherr.IsAccountAlreadyOnline（禁止本包薄 re-export）。
func (e *Engine) ShouldFailFastHandshake(err error) bool {
	if autherr.IsAccountAlreadyOnline(err) {
		// 曾鉴权成功，或已在重连态：持续重试，等待 grace/半死顶替或旧会话释放。
		if e.hasAuthOKOnce() || e.isReconnecting() {
			logger.Info("账号已在线（曾连接/重连中），继续自动重连等待会话顶替…")
			return false
		}
		e.mu.Lock()
		e.onlineRejects++
		n := e.onlineRejects
		e.mu.Unlock()
		if n >= accountOnlineMaxRetries {
			logger.Warn("账号已在线，已达有限重试上限 %d，停止自动重连（可手动重连；若反复出现请检查服务端 grace/半死顶替）", accountOnlineMaxRetries)
			return true
		}
		logger.Info("账号已在线，有限重试 %d/%d（等待 grace 顶替或旧会话释放）", n, accountOnlineMaxRetries)
		return false
	}
	if IsFatalDialError(err) {
		return true
	}
	return IsFatalHandshakeError(err)
}

func (e *Engine) resetOnlineRejects() {
	e.mu.Lock()
	e.onlineRejects = 0
	e.mu.Unlock()
}
