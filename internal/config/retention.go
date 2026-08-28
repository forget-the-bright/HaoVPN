package config

// DefaultRetentionDays 审计日志、连接事件与 history 库默认保留天数。
//
// 为何放在 config 而非 netutil：保留策略属于服务端数据治理，与网络纯函数无关。
// ApplyDefaults 在对应字段 ≤0（或 history 为 0）时填入本常量；-1 表示关闭 history。
const DefaultRetentionDays = 90
