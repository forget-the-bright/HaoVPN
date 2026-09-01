package safeutil

import (
	"context"
	"errors"
)

// IsCanceled 判断 err 是否为 context 取消或截止（Canceled / DeadlineExceeded）。
//
// 用途：NAT/ICS Setup、applyPolicy、via Setup 等路径上，把「用户 Stop/HardRestart」
// 与「真·配置失败」区分开，禁止把取消当成 forward_only「无 SNAT 成功」。
// 关联：netstack.Setup、clientapp.applyPolicy / via_exit；勿与业务错误混用。
func IsCanceled(err error) bool {
	return err != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded))
}

// IsDeadline 判断 err 是否为 context 截止（仅 DeadlineExceeded，不含 Canceled）。
//
// 用途：GUI 登录超时文案（「连接超时」）与用户主动取消区分；Canceled 不应盖成超时提示。
// 实现：errors.Is，兼容 wrap；禁止用 Error() 字符串比对（文案变更会静默失效）。
// 关联：clientgui/login_fail.go；更宽的取消判定用 IsCanceled。
func IsDeadline(err error) bool {
	return err != nil && errors.Is(err, context.DeadlineExceeded)
}

// Check 若 ctx 非 nil 且已 Done，返回 ctx.Err()；否则返回 nil。
//
// 参数：ctx — 可为 nil（视为未取消，返回 nil），便于可选 Abort 挂点。
// 返回：context.Canceled 或 DeadlineExceeded；调用方应打业务日志键（如 policy_apply aborted）。
// 副作用：无（不写日志，避免与调用方日志键重复）。
func Check(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
