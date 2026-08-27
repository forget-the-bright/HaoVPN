// Package crypto 封装隧道密钥对与载荷加解密（X25519 + ChaCha20-Poly1305 + 防重放窗口）。
//
// 被 tunnel 握手与 sessionmgr 报文转发使用；底层算法来自 wireguard-go 思路。
package crypto
