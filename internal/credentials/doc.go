// Package credentials 封装 Windows 服务账号 DPAPI 凭据存取（LocalMachine）。
//
// 上游：clientapp ResolveCredentials（Windows 服务无交互启动）。
// 下游：fileutil、Windows DPAPI API。
// 并发：Load/Save 文件级；服务安装路径单进程写入。
// 不变量：凭据文件权限须受限；非 Windows 平台为 stub/no-op。
package credentials
