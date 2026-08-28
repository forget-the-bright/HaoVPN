// Package crypto 封装隧道密钥对与载荷加解密（X25519 + ChaCha20-Poly1305 + 防重放窗口）。
//
// 上游：tunnel 握手建会话；sessionmgr 转发 TUN 报文加解密。
// 下游：wireguard-go 风格密码学实现（wg_crypto.go）。
// 并发：Session 非线程安全；每连接独立 Session，由 sessionmgr 单 goroutine 读写。
// 不变量：私钥/明文不得写日志；Decrypt 失败丢弃报文。
package crypto
