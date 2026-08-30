// Package vpnaccount 封装 VPN 账号 IP 模式与 Web 开户/策略变更业务逻辑。
//
// 关键文件：
//   service.go — EnsureVPNIP、DefaultAllowedIPs、租约清理
//   provision.go — Web 开户 ProvisionWebAccount
//   patch.go — ApplyVPNPatch（校验+写库+踢线）
//   enable.go — SetAccountEnabled
//   delete.go — DeleteAccount、PlanVPNPatch
//   peer_apply.go — PeerPolicyApplier（托管路由/互访脏标记与应用生效）
//   peer_policy.go — ResolveClientPolicy（握手下发互访/via）
//
// 上游：api（Web 开户/PATCH/peers apply）、tunnel（握手后 IP 分配）。
// 下游：persist、ippool、security（私钥）；OnKickUser / PeerPolicyApplier.Kick 由 serverapp/api 注入 sessionmgr。
// 并发：Service 无内部锁；PeerPolicyApplier 自带 mu；同一账号操作由调用方串行或依赖 SQLite。
// 不变量：IP 分配/回收/PATCH/启禁须经本包；peer 应用生效经 PeerPolicyApplier；api 不直接 UpdateVPNFields/SetUserEnabled。
package vpnaccount
