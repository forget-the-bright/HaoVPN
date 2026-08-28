// Package clientapp CLI/GUI 共用 VPN 拨号引擎。
//
// 关键文件：
//   engine.go — Start/Stop、状态机、onConnect（TLS 后握手+策略）、杀开关
//   runtime.go — TUN 设备、路由、DNS 运行时（applyPolicy/readLoop）
//   credentials.go — yaml/DPAPI/交互式凭据解析
//   bootstrap.go — CLI 启动封装（RunCLI/StopCLI）
//   service_windows.go — Windows 服务 install/SCM 入口
//
// 关联：internal/clientgui 持有 Engine；config.SaveClient / BuildClientExportYAML 管 YAML；
// singleinstance 由 cmd 入口抢锁，服务路径亦 Acquire。
// 上游：cmd/client、cmd/client-gui、internal/clientgui。
// 下游：transport、tunnel、netstack、tun、security、config。
// 并发：Engine 持 mu 保护状态；transport 与 TUN readLoop 各 goroutine。
// 不变量：断线先 EnableKillSwitch 再清路由；策略以握手应答为准。
package clientapp
