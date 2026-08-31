// Package vpnaccount 封装 VPN 账号 IP 模式与 Web 开户/策略变更业务逻辑。
//
// 关键文件：
//   service.go — EnsureVPNIP、DefaultAllowedIPs、租约清理
//   provision.go — Web 开户 ProvisionWebAccount
//   patch.go — ApplyVPNPatch（校验+写库+踢线）
//   enable.go — SetAccountEnabled
//   delete.go — DeleteAccount、PlanVPNPatch
//   peer_apply.go — PeerPolicyApplier（脏标记与应用生效）
//   peer_write.go — 托管路由/互访写用例（Create/Delete/Replace/Add/Remove + 标脏）
//   peer_policy.go — ResolveClientPolicy（握手下发互访/via）
//   errors.go — 领域哨兵（账号/via/peer 路由）
//
// 上游：api（Web 开户/PATCH/peers）、tunnel（握手后 IP 分配）。
// 下游：persist、ippool、security（私钥）；OnKickUser / PeerPolicyApplier.Kick 由 serverapp/api 注入 sessionmgr。
// 并发：Service 无内部锁；PeerPolicyApplier 自带 mu；同一账号操作由调用方串行或依赖 SQLite。
// 不变量：IP 分配/回收/PATCH/启禁须经本包；peer 写与应用生效经 PeerPolicyApplier；api 不直接 UpdateVPNFields/SetUserEnabled/InsertPeerRoute。
package vpnaccount
