package winnet

import "sync"

// Options Windows 网卡加速运行时开关（由 client.yaml windows 段注入）。
//
// UseIPHelper — 读/写优先 IP Helper；失败回退 netsh/route/Go net。
type Options struct {
	UseIPHelper bool
}

var (
	optsMu sync.RWMutex
	opts   = Options{UseIPHelper: true}
)

// Configure 由客户端启动时根据 config.ClientWindowsSection 注入；可重复调用。
func Configure(o Options) {
	optsMu.Lock()
	opts = o
	optsMu.Unlock()
}

// currentOptions 返回当前开关快照。
func currentOptions() Options {
	optsMu.RLock()
	defer optsMu.RUnlock()
	return opts
}

// UseIPHelperEnabled 当前是否优先 IP Helper。
func UseIPHelperEnabled() bool {
	return currentOptions().UseIPHelper
}

// Shutdown 进程退出前挂点（历史曾关闭常驻 PowerShell；该加速面已删除）。
//
// 为何保留：Engine.Stop / GUI 退出路径已调用；未来若需进程级清理（缓存、子进程）可挂此函数。
// 当前：空操作，无副作用；禁止再引入常驻 powershell 主机。
func Shutdown() {}
