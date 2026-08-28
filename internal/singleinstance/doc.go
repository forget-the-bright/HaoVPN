// Package singleinstance 保证同一台机器上仅有一个 HaoVPN 客户端进程（CLI/GUI/Windows 服务互斥）。
//
// 实现：127.0.0.1 固定哈希端口 TCP Listen；重复启动 Dial 成功即表示已有实例。
// 该方式跨平台，且在 Windows 上不受 UAC 管理员/非管理员隔离影响（优于仅文件锁）。
package singleinstance
