package clientapp

// AttachDataplaneHook 注册鉴权成功后 TUN/路由失败回调（GUI/CLI 共用注册模式）。
//
// 须在 Start 前调用。GUI 传入 fyne.Do 包装；CLI 传入 stderr + StopCLI。
func AttachDataplaneHook(eng *Engine, fn func(msg string)) {
	if eng == nil || fn == nil {
		return
	}
	eng.setOnDataplaneFailed(fn)
}
