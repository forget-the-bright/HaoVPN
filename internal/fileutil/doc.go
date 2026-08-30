// Package fileutil 提供与业务无关的文件系统小工具。
//
// 上游：config、logger、security、credentials、tunnel、wintundll、platform、autostart、health 等。
// 下游：仅标准库 os/path/filepath（本包不得 import logger，避免与 logger→fileutil 循环）。
// 能力：
//   EnsureParentDir / WriteFileAtomic — 敏感配置与密钥原子写；
//   ExecutableDir — 解析 exe 旁路径；
//   Exists — 证书/健康检查/自启状态探测；
//   AbsPair — autostart linux/darwin 共用绝对路径解析；
//   CheckWorldReadable — Unix 权限过宽检测（由 health 打 Warn）。
// 不变量：EnsureParentDir 对空路径或已存在目录返回 nil；WriteFileAtomic 失败不留下半截目标文件。
package fileutil
