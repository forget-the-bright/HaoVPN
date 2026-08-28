package persist

// DefaultIPLeaseSec dynamic_lease 未指定租约秒数时的默认值。
//
// 与 schema.sql 中 users.ip_lease_sec 的 DEFAULT 86400 保持一致；
// 读写路径（CreateVPNAccount、扫描回填、vpnaccount 开户/断线）须引用本常量，禁止散落魔法数。
const DefaultIPLeaseSec = 86400
