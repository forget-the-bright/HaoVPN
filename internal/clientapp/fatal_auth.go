package clientapp

import (
	"errors"
	"strings"

	"haovpn/internal/auth"
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
	if IsAccountAlreadyOnline(err) {
		return false
	}
	for _, target := range fatalAuthSentinels {
		if errors.Is(err, target) {
			return true
		}
	}
	msg := err.Error()
	for _, target := range fatalAuthSentinels {
		if t := target.Error(); t != "" && strings.Contains(msg, t) {
			return true
		}
	}
	for _, s := range fatalAuthLegacySubstrs {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// IsAccountAlreadyOnline 判断是否为「同账号已在其他设备在线」类错误。
//
// 优先 errors.Is(auth.ErrAccountAlreadyOnline)，避免依赖 sessionmgr（分层：clientapp 不引会话路由包）。
func IsAccountAlreadyOnline(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, auth.ErrAccountAlreadyOnline) {
		return true
	}
	msg := err.Error()
	if strings.Contains(msg, auth.ErrAccountAlreadyOnline.Error()) {
		return true
	}
	return strings.Contains(msg, "已在其他设备在线")
}

// ShouldFailFastHandshake 结合 Engine 有限重试，决定是否因本次握手失败停止重连。
func (e *Engine) ShouldFailFastHandshake(err error) bool {
	if IsAccountAlreadyOnline(err) {
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
	return IsFatalHandshakeError(err)
}

func (e *Engine) resetOnlineRejects() {
	e.mu.Lock()
	e.onlineRejects = 0
	e.mu.Unlock()
}

// fatalAuthSentinels 与 auth 导出哨兵对齐（不含 account_online，见有限重试）。
var fatalAuthSentinels = []error{
	auth.ErrBadCredentials,
	auth.ErrAccountDisabled,
	auth.ErrLoginLocked,
	auth.ErrMustChangePassword,
	auth.ErrNoVPN,
	auth.ErrPasswordRequired,
	auth.ErrUsePasswordLogin,
	auth.ErrInvalidHandshake,
}

// fatalAuthLegacySubstrs 历史/部分截断文案兜底（不含「已在其他设备在线」）。
var fatalAuthLegacySubstrs = []string{
	"须修改密码",
	"缺少账号密码",
	"登录已锁定",
	"IP 已锁定",
}
