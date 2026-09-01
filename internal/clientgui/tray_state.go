package clientgui

import (
	"fmt"

	"haovpn/internal/clientapp"
)

// trayPresentation 由 Engine 状态映射到托盘图标与菜单缓存键。
type trayPresentation struct {
	Kind    trayKind
	MenuKey string
}

// trayPresentationFromEngine 将 Engine.State/LastError 映射为托盘展示（图标 + menuKey）。
//
// eng==nil 时返回 Idle；与 formatTrayTooltip 的 State 分支语义对齐，供 syncTrayFromEngine 复用。
func trayPresentationFromEngine(eng *clientapp.Engine) trayPresentation {
	if eng == nil {
		return trayPresentation{Kind: trayKindIdle, MenuKey: ""}
	}
	st := eng.State()
	errMsg := eng.LastError()
	switch {
	case st == clientapp.StateConnected:
		return trayPresentation{
			Kind:    trayKindConnected,
			MenuKey: fmt.Sprintf("up:%s:a%d:m%d", eng.VPNIP(), len(eng.AllowedIPs()), len(eng.ManagedRoutes())),
		}
	case st == clientapp.StateConnecting || st == clientapp.StateReconnecting:
		return trayPresentation{Kind: trayKindConnecting, MenuKey: "connecting"}
	case errMsg != "" && st == clientapp.StateIdle:
		return trayPresentation{Kind: trayKindError, MenuKey: "err"}
	default:
		return trayPresentation{Kind: trayKindIdle, MenuKey: "idle"}
	}
}
