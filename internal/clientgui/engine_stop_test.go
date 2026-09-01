package clientgui

import (
	"strings"
	"testing"

	"haovpn/internal/clientapp"
)

// TestBeginEngineOpPreventsReentry 忙状态时二次 begin 失败，end 后可再 begin。
func TestBeginEngineOpPreventsReentry(t *testing.T) {
	u := &uiApp{}
	if !u.beginEngineOp() {
		t.Fatal("首次应成功")
	}
	if u.beginEngineOp() {
		t.Fatal("忙时不得再 begin")
	}
	u.endEngineOp()
	if !u.beginEngineOp() {
		t.Fatal("end 后应可再 begin")
	}
}

// TestTakeEngineClearsField takeEngine 取出后 eng 应为 nil。
func TestTakeEngineClearsField(t *testing.T) {
	u := &uiApp{}
	if u.takeEngine() != nil {
		t.Fatal("空 eng 应 nil")
	}
}

// TestSetGetEngineLocked set/get/clearEngineIf 与 take 一致。
func TestSetGetEngineLocked(t *testing.T) {
	u := &uiApp{}
	fake := &clientapp.Engine{}
	u.setEngine(fake)
	if u.getEngine() != fake {
		t.Fatal("get 应返回 set 的实例")
	}
	if !u.isSameEngine(fake) {
		t.Fatal("isSameEngine")
	}
	if !u.clearEngineIf(fake) || u.getEngine() != nil {
		t.Fatal("clearEngineIf 应清空")
	}
}

// TestSetEngineOpBusyUINilSafe 无按钮指针时不 panic。
func TestSetEngineOpBusyUINilSafe(t *testing.T) {
	u := &uiApp{}
	u.setEngineOpBusyUI(true)
	u.setEngineOpBusyUI(false)
}

// TestTrayTooltipInputStickyWinsOverBusy 登录失败清理中 busy+sticky：tip 须保留错误，勿「正在断开」。
func TestTrayTooltipInputStickyWinsOverBusy(t *testing.T) {
	u := &uiApp{}
	u.setTrayStickyErr("连不上服务端")
	if !u.beginEngineOp() {
		t.Fatal("begin")
	}
	in := u.trayTooltipInputNow()
	if in.Phase == trayTipDisconnecting {
		t.Fatal("sticky 存在时 busy 不得盖成正在断开")
	}
	if in.LastError != "连不上服务端" {
		t.Fatalf("LastError=%q", in.LastError)
	}
	got := formatTrayTooltip(in)
	if !strings.Contains(got, "错误:") || !strings.Contains(got, "连不上") {
		t.Fatalf("tip=%q", got)
	}
	u.clearTrayStickyErr()
	in = u.trayTooltipInputNow()
	if in.Phase != trayTipDisconnecting {
		t.Fatalf("无 sticky 时应正在断开 phase=%v", in.Phase)
	}
	u.endEngineOp()
}

// TestTrayTooltipInputClearsSticky 清 sticky 后 tip 回到「未连接」。
func TestTrayTooltipInputClearsSticky(t *testing.T) {
	u := &uiApp{}
	u.setTrayStickyErr("x")
	u.clearTrayStickyErr()
	in := u.trayTooltipInputNow()
	if in.LastError != "" {
		t.Fatalf("LastError=%q", in.LastError)
	}
	if got := formatTrayTooltip(in); !strings.Contains(got, "未连接") {
		t.Fatalf("%q", got)
	}
}

// TestApplyLogoutFeedbackPendingMessage 数据面失败登出后须 sticky 错误，非裸「未连接」。
func TestApplyLogoutFeedbackPendingMessage(t *testing.T) {
	u := &uiApp{pendingLogoutMsg: "数据面配置失败"}
	u.applyLogoutFeedback()
	if u.pendingLogoutMsg != "" {
		t.Fatal("pending 应已消费")
	}
	in := u.trayTooltipInputNow()
	if in.LastError != "数据面配置失败" {
		t.Fatalf("sticky=%q", in.LastError)
	}
	u.trayMu.Lock()
	kind := u.trayKind
	u.trayMu.Unlock()
	if kind != trayKindError {
		t.Fatalf("trayKind=%v want error", kind)
	}
}

// TestApplyLogoutFeedbackIdle 主动退出无 pending：托盘 idle。
func TestApplyLogoutFeedbackIdle(t *testing.T) {
	u := &uiApp{}
	u.setTrayStickyErr("旧错误")
	u.applyLogoutFeedback()
	in := u.trayTooltipInputNow()
	if in.LastError != "" {
		t.Fatalf("应清空 sticky got=%q", in.LastError)
	}
}

