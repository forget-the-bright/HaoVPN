// Package config 负责 server.yaml / client.yaml 的加载、校验、默认值与首次启动模板生成。
//
// 关键 API：LoadServer、LoadClient、ClientConfig.ApplyDefaults、LogSection.InitGlobal、ResolveClientConfigPath。
// 网络校验委托 netutil；TLS 构建在 security 包。
package config
