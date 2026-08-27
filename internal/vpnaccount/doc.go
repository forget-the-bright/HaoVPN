// Package vpnaccount 封装 VPN 账号 IP 模式与 Web 开户/策略变更业务逻辑。
//
// 上游：api（Web 开户/PATCH）、tunnel（握手鉴权后 IP 分配）。
// 下游：persist（用户表）、ippool（IP 池）、security（私钥加解密）。
// 并发：Service 方法由 HTTP/隧道 goroutine 调用；依赖 store 线程安全。
// 不变量：IP 分配与回收须经本包，api 不直接操作 IP 池 SQL；避免 api↔tunnel 循环依赖。
package vpnaccount
