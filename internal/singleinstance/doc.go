// Package singleinstance 保证同一台机器上仅有一个 HaoVPN 客户端进程（CLI/GUI/Windows 服务互斥）。
//
// 锁文件位于系统临时目录；AcquireClient 被 cmd/client 与 cmd/client-gui 入口调用。
package singleinstance
