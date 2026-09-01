package clientapp

import (
	"context"
	"errors"
	"time"

	"haovpn/internal/config"
	"haovpn/internal/logger"
)

// DefaultFirstAuthTimeout 交互式 CLI/GUI 首连 WaitConnected 超时（与登录页一致）。
const DefaultFirstAuthTimeout = 45 * time.Second

// DefaultGUIRunOptions GUI 登录建 Engine 用：FailFast、不重复预热（run.go 已 StartWarmupAsync）。
//
// onDataplane 可为 nil，由 clientgui attachDataplaneHook 单独挂（须 fyne.Do）。
func DefaultGUIRunOptions(onDataplane func(string)) RunOptions {
	return RunOptions{
		Mode:            BootstrapGUI,
		WarmupTun:       false,
		FailFastFirst:   true,
		WaitFirstAuth:   0,
		OnDataplaneFail: onDataplane,
	}
}

// PrepareEngine 按 RunOptions 预热并构造 Engine（挂 hook），不 Start。
//
// 供 CLI bootstrap 与 GUI tryConnect 共用；避免 NewEngine/SetFailFast 双份维护。
func PrepareEngine(cfg *config.ClientConfig, creds Credentials, opts RunOptions) (*Engine, error) {
	if cfg == nil {
		return nil, errors.New("PrepareEngine: 配置为空")
	}
	if opts.WarmupTun {
		StartWarmupAsync(cfg.Tun.Name)
	}
	eng := NewEngine(cfg)
	eng.SetCredentials(creds)
	if opts.FailFastFirst {
		eng.SetFailFast(true)
	}
	if opts.OnDataplaneFail != nil {
		AttachDataplaneHook(eng, opts.OnDataplaneFail)
	}
	if opts.OnUserWarn != nil {
		eng.SetUserWarnSink(opts.OnUserWarn)
	}
	return eng, nil
}

// StartAndWaitFirstAuth 启动 Engine 并阻塞至首连鉴权结果；失败时 Stop 并返回格式化错误。
//
// 成功仅表示账号握手通过（同 WaitConnected 语义）；数据面失败走 OnDataplaneFailed。
func StartAndWaitFirstAuth(ctx context.Context, eng *Engine) error {
	if eng == nil {
		return errors.New("StartAndWaitFirstAuth: engine 为空")
	}
	if err := eng.Start(); err != nil {
		return errors.New(FormatConnectFailure(err, "", nil))
	}
	err := eng.WaitConnected(ctx)
	if err != nil {
		msg := FormatConnectFailure(err, eng.LastError(), ctx.Err())
		logger.Warn("client_bootstrap first_auth=fail msg=%s", msg)
		eng.Stop()
		return errors.New(msg)
	}
	logger.Info("client_bootstrap first_auth=ok")
	return nil
}
