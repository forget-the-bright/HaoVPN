// Package serverapp 编排服务端进程：库、隧道、TUN/NAT、管理 API 与优雅关闭。
//
// 关键文件（第十六轮同包拆分）：
//   engine.go — Run() 调用各 boot* 阶段
//   engine_boot.go — bootContext 等编排辅助
//   boot_persist.go / boot_ippool.go / boot_session.go —
//   boot_tun.go / boot_tunnel.go / boot_api.go — 分阶段启动
//   engine_shutdown.go — 优雅关闭
//
// 上游：cmd/server 加载 config 后 Engine.Run()。
// 下游：api、tunnel、transport、sessionmgr、vpnaccount、persist、netstack、tun、probedefense、maintenance。
// 并发：Run 阻塞主 goroutine；safeutil 管理子生命周期。
// 不变量：BindCheck 失败 Fatal；关闭先停 API 再停隧道与 TUN；有 Probe Guard 即挂 Accept。
package serverapp
