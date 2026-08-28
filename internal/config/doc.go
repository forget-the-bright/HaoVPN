// Package config 负责 server.yaml / client.yaml 的加载、校验、默认值与首次启动模板生成。
//
// 关键 API：
//   LoadServer / LoadClient — 不存在则原子写模板（fileutil.WriteFileAtomic）
//   ClientConfig.ApplyDefaults / ServerConfig.ApplyDefaults — 缺省填充
//   BuildClientExportYAML — 服务端导出客户端 YAML（与模板字段对齐；api 薄封装）
//   SaveClient — GUI yaml.Node 局部 patch（保留注释、剥离 legacy peer）
//   ResolveClientConfigPath / LoadClientOrDefaults — exe 旁路径与内存回退
//   DefaultRetentionDays — 审计/事件/history 默认保留天数
//
// 网络校验委托 netutil；TLS 构建在 security；敏感写盘统一原子写。
package config
