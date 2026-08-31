package probedefense

import (
	"haovpn/internal/autherr"
)

// ClassifyHandshakeReject 将握手失败映射为探针 signature（委托 autherr 统一分类）。
func ClassifyHandshakeReject(err error) string {
	switch autherr.Classify(err) {
	case autherr.CategoryAccountOnline:
		return SigAccountOnline
	case autherr.CategoryAuthFailed:
		return SigAuthFailed
	case autherr.CategorySourceDenied:
		return SigSourceDeny
	default:
		return SigHandshakeReject
	}
}
