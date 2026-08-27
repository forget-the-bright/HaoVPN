// Package netstack 管理 TUN 侧路由、DNS 与杀开关（按平台分文件实现）。
//
// 子模块划分（同包内按文件名组织，避免 import 循环）：
//   - route_*.go：分流路由增删（Windows netsh / Linux ip / Darwin route）
//   - dns_*.go：TUN 接口 DNS 设置
//   - killswitch_*.go：断线阻断工控网段
//
// 依赖 winnet（Windows 网卡解析）与 netutil（CIDR 校验），不直接依赖 tun 包。
package netstack
