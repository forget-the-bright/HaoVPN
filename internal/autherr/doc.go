// Package autherr 握手/登录/拨号错误的统一分类（clientapp 停重连 UX 与 probedefense 探针签名共用）。
//
// 依赖 auth 哨兵与 transport.ErrIPBanned；禁止 import api、clientapp、probedefense、serverapp。
// 关联：clientapp/fatal_auth.go、probedefense/classify_handshake.go、clientapp/dial_errors.go。
package autherr
