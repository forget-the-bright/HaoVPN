// Package persist 提供 SQLite 持久化：用户/VPN、托管路由、LAN 注册、审计与 schema。
//
// 关键文件（第十六轮同包拆分）：
//   store.go / constants.go / users.go — Store、租约常量、账号 CRUD
//   peer_types.go / peer_access.go / peer_routes.go / peer_route_normalize.go —
//     互访与托管路由；成员校验（须存在且 HasVPN）；UnionMemberUserIDs；SymmetricDiffUserIDs
//   dns_types.go / dns_servers.go / dns_normalize.go / dns_seed.go —
//     托管 DNS（members−excludes）；YAML seed；DeleteUser 级联清绑定；空成员删 manual 行
//   lan_registry.go — 客户端 LAN 广告；host_id 截断；HasLanRegistryMatch
//   migrate_peer_routes.go / query_*.go / security_store.go / settings.go — 迁移、列表、封禁、运行时设置
//
// 上游：serverapp、api、auth、vpnaccount、sessionmgr、tunnel。
// 下游：sqlite、paginate、readmodel、timeutil、netutil。
// 并发：max_open_conns=1；事务经 DB() 自行管理。
// 不变量：schema.sql 为唯一表结构；DeleteUser 级联清理子表；成员/via 错误中文稳定文案。
package persist
