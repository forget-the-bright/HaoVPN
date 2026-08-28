// Package config 负责 server.yaml / client.yaml 的加载、校验、默认值与导出。
//
// 关键文件：
//   config.go / client.go / server.go — Load、Validate、ApplyDefaults
//   client_export.go — BuildClientExportYAML
//   client_yaml_patch.go — SaveClient
//   yaml_node.go — yaml.Node 局部 patch 原语
//   paths.go — DefaultServerCertPath、ResolveServerCertPath
//   retention.go — DefaultRetentionDays
//
// 上游：cmd/*、serverapp、clientapp、clientgui、api 导出。
// 下游：netutil、fileutil、brand。
// 并发：配置加载后只读；SaveClient 原子写须避免并发写同一路径。
// 不变量：敏感写盘经 fileutil.WriteFileAtomic；VERSION 不在此包硬编码。
package config
