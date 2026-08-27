// Package serverapp 服务端进程编排：数据库、隧道、TUN/NAT、管理 API 与优雅关闭。
//
// 关键文件：engine.go — Run() 八阶段启动流水线。
//
// 上游：cmd/server 加载 config 后调用 Engine.Run()。
// 下游：api、tunnel、transport、sessionmgr、vpnaccount、persist、netstack、tun。
// 并发：Run 阻塞主 goroutine；safeutil.Shutdown 管理子 goroutine 生命周期。
// 不变量：BindCheck 失败 Fatal；关闭时先停 API 再停隧道与 TUN。
package serverapp
