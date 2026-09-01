package netstack

// ICSLifecycle 拆除 via / 数据面时对 Windows ICS 的策略。
//
// 为何不用裸 bool：历史上 keepICS 一名两义（HardRestart 保 137 vs via 重建暂不 Disable），
// 阅读成本高；枚举把意图钉在类型上，避免 Logout 误传 true 留下共享。
//
// 关联：
//   - clientapp.Stop / StopKeepICS → runtime.close → teardownViaExit
//   - Stack.Teardown / TeardownKeepICS
//   - 有 137 时下次 setupICSWithPublicIf → reuse_live（见 ics_enable_windows.go）
type ICSLifecycle int

const (
	// ICSDisable Logout / 关闭 via：走 DisableICSSession（拆共享，可十余秒）。
	ICSDisable ICSLifecycle = iota
	// ICSPreserve HardRestart / via 指纹重建：不 Disable，保留 TUN 上 192.168.137.*。
	ICSPreserve
)

// Preserve 是否保留 ICS（true=ICSPreserve）。
func (p ICSLifecycle) Preserve() bool {
	return p == ICSPreserve
}

// LogLabel 供结构化日志 keep_ics=… 与旧字段兼容。
func (p ICSLifecycle) LogLabel() string {
	if p.Preserve() {
		return "preserve"
	}
	return "disable"
}
