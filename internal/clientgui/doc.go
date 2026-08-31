// Package clientgui Fyne 桌面客户端 UI（登录、主窗口、托盘、日志面板）。
//
// 关键文件：
//
//	run.go — 创建 Fyne 应用、后台 WarmupTun（经 clientapp，禁止直接 import tun）、进入事件循环
//	fyne_meta.go — SetMetadata Migrations.fyneDo（纯 go build 不读 FyneApp.toml）
//	tray.go — 托盘菜单/图标；Disconnecting；手动重连入口 reconnectVPN
//	tray_tooltip.go — tip 预算 63 UTF-16（无 NOTIFYICON_VERSION_4）；IP→连接自→主机
//	login.go — 登录窗；NewEngine + SetFailFast(true)（与 HardRestart 语义分离）
//	reconnect_dns.go — 调用 clientapp.HardRestart（Stop+DNS settle+新 Engine）；UI 经 fyne.Do
//	notice.go — 单实例/致命提示
//	app.go — 主窗口、轮询、登出（立刻 Disconnecting tip）与 shutdown；服务凭据经 clientapp.SaveServiceCredentials
//	log.go — 日志缓冲与 logger sink
//	tray_routes.go — 托盘托管路由展示
//	tray_config.go — 托盘配置菜单（自启/服务；SaveServiceCredentials 门面）
//	engine_stop.go — 后台 Stop；beginEngineOp + 按钮 Disable 防连点
//
// 上游：cmd/client-gui；构建须 -tags migrated_fynedo。
// 下游：clientapp、autostart、config、netutil、safeutil、brand、singleinstance、platform；禁止 tun/winnet/credentials。
//
// 重连契约：Soft 在 clientapp/transport；Hard 仅 HardRestart；禁止 GUI 第三套编排。
// 线程模型（fyneDo）：后台改 UI 须 fyne.Do；重活禁止塞进 Do；托盘悬停经 systray.SetTooltip。
package clientgui
