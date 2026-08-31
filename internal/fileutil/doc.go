// Package fileutil 提供与业务无关的文件系统小工具。
//
// 上游：config、logger、security、credentials、tunnel、wintundll、platform、autostart、health 等。
// 下游：仅标准库 os/path/filepath（本包不得 import logger，避免与 logger→fileutil 循环）。
// 能力：
//   EnsureParentDir / WriteFileAtomic — 敏感配置与密钥原子写；
//   ExecutableDir — 解析 exe 旁路径；
//   Exists — 证书/健康检查/自启状态探测；
//   AbsPair — autostart linux/darwin 共用绝对路径解析；
//   CheckDirWritable — health 验证 DB 目录可写（不重复 MkdirAll）；
//   CheckWorldReadable — Unix 权限过宽检测（由 health 打 Warn）。
//
// 目录权限约定：
//   0o755 — 普通数据目录（logs、data、数据库父目录）；
//   0o700 — 敏感目录/文件（encryption_key、security 密钥路径 EnsureParentDir）。
package fileutil
