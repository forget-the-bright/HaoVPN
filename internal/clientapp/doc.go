// Package clientapp CLI/GUI 共用 VPN 拨号引擎。
//
// 关键文件：
//   engine.go — TLS 连接、握手、重连、杀开关主循环
//   runtime.go — TUN 设备、路由、DNS 运行时（applyPolicy/readLoop）
//   credentials.go — yaml/DPAPI/交互式凭据解析
//   bootstrap.go — CLI 启动封装（RunCLI/StopCLI）
//   service_windows.go — Windows 服务 install/SCM 入口
//
// 上游：cmd/client、cmd/client-gui、internal/clientgui。
// 下游：transport、tunnel、netstack、tun、security、config。
// 并发：Engine 持 mu 保护状态；transport 与 TUN readLoop 各 goroutine。
// 不变量：断线先 EnableKillSwitch 再清路由；策略以握手应答为准。
package clientapp
