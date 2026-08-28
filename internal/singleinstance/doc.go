// Package singleinstance 保证客户端 CLI/GUI 单实例（127.0.0.1 TCP 协调，非文件锁）。
//
// 关键文件：lock.go（Acquire/Release）、coord.go（端口哈希与探测报文）。
// 上游：cmd/client、cmd/client-gui、clientgui.Run。
// 下游：net 标准库 TCP。
// 并发：Acquire 阻塞至获锁或已有实例响应；Release 在进程退出时调用。
// 不变量：重复启动须提示用户而非静默失败；协调端口由可执行路径哈希派生。
package singleinstance
