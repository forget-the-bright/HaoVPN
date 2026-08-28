// Package netstack 管理 TUN 侧路由、DNS 与杀开关（按平台分文件实现）。
//
// 关键文件：route_*.go、dns_*.go、killswitch_*.go、common.go。
// 上游：clientapp runtime、serverapp TUN/NAT Setup。
// 下游：winnet（Windows 网卡）、netutil（CIDR）、platform（无窗口子进程）。
// 并发：Setup/Teardown 由调用方串行；路由变更非线程安全须单 goroutine 编排。
// 不变量：不 import tun；断线杀开关先于清路由（clientapp 编排）。
package netstack
