// Package autherr 握手/登录/拨号错误的统一分类（clientapp 停重连 UX 与 probedefense 探针签名共用）。
//
// 依赖：auth 哨兵、dialerr 拨号哨兵（ErrIPBanned / ErrSourceDenied / ErrPlaintextBeforeTLS）。
// 禁止 import transport、api、clientapp、probedefense、serverapp、tunnel。
//
// 线上契约：HandshakeCode / FromHandshakeCode 与 handshake_err JSON 的 code 字段对应；
// 旧服务端无 code 时客户端仍用 Error 文案 + Classify 子串兜底（子串表与 Is* 共用，勿双份维护）。
//
// 关联：clientapp/fatal_auth.go、dial_errors.go（直接调本包 Is*，无薄 re-export）；
// probedefense/classify_handshake.go；tunnel/handshake*.go / handshake_reject.go。
package autherr
