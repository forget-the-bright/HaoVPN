package dialerr

import (
	"errors"
	"fmt"
	"strings"
)

// tlsBadRecordSubstr crypto/tls 在首包非 TLS 记录时的固定英文片段（大小写不敏感匹配）。
//
// 为何集中：transport Dial 映射与 probedefense.ClassifyTLSError 共用，避免两处各写一遍子串。
const tlsBadRecordSubstr = "first record does not look like a tls"

// IsTLSBadRecordMsg 判断错误文案是否为「首包不像 TLS」（含 wrap）。
//
// 参数：err — 任意错误；nil 返回 false。
// 返回：true 表示 crypto/tls bad-record 类文案。
func IsTLSBadRecordMsg(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), tlsBadRecordSubstr)
}

// ClassifyTLSHandshakeErr 将「首包非 TLS」映射为 ErrPlaintextBeforeTLS（不直接断言封禁）。
//
// 说明：晚到的 HAOVPN banner、或连到 HTTP/其它端口，都会触发同一 crypto/tls 文案；
// 调用方应用 FormatDialError 给出「封禁或连错端口」双因提示，仅在明确读到 banner 时用 ErrIPBanned。
func ClassifyTLSHandshakeErr(err error) error {
	if err == nil {
		return nil
	}
	if IsTLSBadRecordMsg(err) {
		return fmt.Errorf("%w: %v", ErrPlaintextBeforeTLS, err)
	}
	return err
}

// IsFatalDialError 拨号/TLS 阶段应停止自动重连的错误（封禁、源拒绝、明文拒绝）。
//
// ErrClosedBeforeTLS 可重试，不算 fatal。
func IsFatalDialError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrIPBanned) ||
		errors.Is(err, ErrSourceDenied) ||
		errors.Is(err, ErrPlaintextBeforeTLS)
}

// matchRejectBannerPrefix 按 TrimSpace 后前缀识别拒绝码；无法识别返回 nil。
//
// 为何私有共用：ClassifyRejectBannerLine（未知则 error）与 Bytes（未知则 nil）只差「未知」语义。
func matchRejectBannerPrefix(s string) error {
	s = strings.TrimSpace(s)
	switch {
	case strings.HasPrefix(s, bannerPrefixIPBanned):
		return ErrIPBanned
	case strings.HasPrefix(s, bannerPrefixSourceDenied):
		return ErrSourceDenied
	default:
		return nil
	}
}

// ClassifyRejectBannerLine 解析一行 TLS 前拒绝码（TrimSpace 后前缀匹配）。
//
// 参数 line — 含或不含 \r\n 均可。
// 返回对应哨兵；无法识别时返回带原文的 error（非哨兵）。
func ClassifyRejectBannerLine(line string) error {
	if err := matchRejectBannerPrefix(line); err != nil {
		return err
	}
	return fmt.Errorf("unexpected server preamble: %s", strings.TrimSpace(line))
}

// ClassifyRejectBannerBytes 对已缓冲字节做前缀识别；无法识别时返回 nil（非错误）。
//
// 用于 peek 超时后缓冲内可能已有部分 banner 的场景。
func ClassifyRejectBannerBytes(b []byte) error {
	return matchRejectBannerPrefix(string(b))
}
