// Package clientgui Fyne 桌面客户端 UI（登录、主窗口、托盘、日志面板）。
//
// 关键文件：
//
//	run.go — 创建 Fyne 应用、clientapp.StartWarmupAsync、进入事件循环（UI 专属，引擎逻辑在 clientapp）
//	fyne_meta.go — SetMetadata Migrations.fyneDo（纯 go build 不读 FyneApp.toml）
//	tray.go — 托盘菜单/图标；Disconnecting；手动重连入口 reconnectVPN
//	tray_tooltip.go — tip 预算 63 UTF-16（无 NOTIFYICON_VERSION_4）；IP→连接自→主机
//	login.go — 登录窗；PrepareEngine + StartAndWaitFirstAuth（与 CLI bootstrap 共用契约）
//	login_fail.go — finishLoginFailure（FormatConnectFailure；busy 则 pending logout）
//	reconnect_dns.go — HardRestart + abort；decideHardRestartFinish；orphan Stop；clientapp.AttachDataplaneHook
//	engine_intent.go — pendingIntent / prepareGUIEngine / dataplaneHookFor
//	engine_op_queue.go — 意图队列纯状态机（无 Fyne，表驱动单测）
//	notice.go — ShowAlreadyRunning / ShowFatalNotice
//	service_takeover.go — AskServiceTakeover / StopServiceForTakeover（经 clientapp autostart_facade）
//	admin.go — requireAdmin / UAC 门闩
//	app.go — 主窗口、轮询、登出、finishQuitApp
//	log.go — 日志缓冲与 logger sink
//	tray_routes.go — 托盘托管路由展示
//	tray_config.go — 托盘配置菜单（自启/服务；经 clientapp autostart_facade）
//	tray_state.go — trayPresentationFromEngine（State→托盘图标/menuKey）
//	engine_stop.go — stopEnginesSerial；beginEngineOp + beginNetworkOp + 按钮 Disable
//
// 托盘 tip 状态：engOpBusy 默认「正在断开」；登录失败 sticky 优先于 busy；无 eng 时 trayStickyErr。
// 登录失败：finishLoginFailure 先展示原因，再 beginEngineOp+Stop（禁未清完就 NewEngine）。
// HardRestart：abort 可中止 DNS/Start；Start 失败须 Stop；成功须 attachDataplaneHook。
// busy 期间再点重连/登出/退出：pendingIntent + opGen，禁止「清理与拨号并行、退出被拒」。
// 上游：cmd/client-gui；构建须 -tags migrated_fynedo。
// 下游：clientapp、config、netutil、safeutil、brand、singleinstance、platform；禁止 tun/winnet/credentials/autostart 直接编排。
//
// 重连契约：Soft 在 clientapp/transport；Hard 仅 HardRestart；禁止 GUI 第三套编排。
// 线程模型（fyneDo）：后台改 UI 须 fyne.Do；重活禁止塞进 Do；托盘悬停经 systray.SetTooltip。
package clientgui
