package clientgui

import (
	"strings"

	"haovpn/internal/clientapp"
	"haovpn/internal/logger"
)

// finishLoginFailure 登录失败后的统一收口（须在 UI 线程调用）。
//
// 契约：
//  1. 立刻红字 + 红托盘 + sticky tip（用户先看到原因，不卡「正在连接」）；
//  2. clearEngineIf 后串行 Stop（beginEngineOp）：禁止未清完就 NewEngine，避免双 TUN/ICS；
//  3. busy 期间 tip 仍展示 sticky（见 trayTooltipInputNow），不用「正在断开」盖掉失败原因；
//  4. Stop 完成 endEngineOp 后才重新 Enable「连接」。
func (u *uiApp) finishLoginFailure(eng *clientapp.Engine, msg string) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		msg = clientapp.FormatConnectFailure(nil, "", nil)
	}
	u.stopPoll()
	u.clearEngineIf(eng)
	u.setTrayStickyErr(msg)
	if u.errLbl != nil {
		u.errLbl.SetText(msg)
	}
	u.applyTray(trayKindError, true)
	logger.Warn("gui_login_fail msg=%s", msg)

	if !u.beginEngineOp() {
		// 登出/退出/重连已占 busy：排队 logout 意图保证本 eng 被串行 Stop，勿只 Enable 按钮漏清。
		logger.Warn("gui_login_fail stop_while_busy → pending logout")
		_ = u.setPendingIntent(intentLogout, clientapp.Credentials{})
		// 若 pending 机制未吃到（极少），仍异步 Stop 本实例避免双 TUN
		u.stopEngineAsync(eng, nil)
		return
	}
	u.stopEngineAsync(eng, func() {
		u.endEngineOp()
		u.applyTray(trayKindError, true)
		logger.Info("gui_login_fail cleanup_done")
	})
}
