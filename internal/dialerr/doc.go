// Package dialerr 拨号/TLS 前拒绝的叶子哨兵与 banner 常量（无业务依赖）。
//
// 职责：
//   - TLS 前明文拒绝码 HAOVPN:IP_BANNED / SOURCE_DENIED（Banner* 常量）；
//   - 拨号阶段哨兵 ErrIPBanned / ErrSourceDenied / ErrPlaintextBeforeTLS / ErrClosedBeforeTLS；
//   - IsFatalDialError、ClassifyTLSHandshakeErr、ClassifyRejectBannerLine/Bytes（共用前缀匹配）。
//
// 为何独立成包：避免 autherr→transport、tunnel/probedefense 各持一枚 ErrSourceDenied；
// 下游 transport（I/O）、autherr（分类）、netutil（源白名单 wrap）、clientapp（UX）共用同一哨兵。
// Error() 统一中文主句；对外判定以 errors.Is 为准。
//
// 上游：无（叶子）。下游：transport、autherr、netutil、probedefense、clientapp。
// 禁止 import api、tunnel、probedefense、clientapp、serverapp、auth。
// 禁止他包再 new 同义哨兵或薄 re-export 本包哨兵（autherr.ErrSourceDenied 同指针别名除外）。
package dialerr
