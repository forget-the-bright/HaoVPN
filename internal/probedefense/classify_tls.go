package probedefense

import (
	"encoding/binary"
	"strings"
)

// ClassifyTLSError 将 TLS/读错误映射为 signature。
func ClassifyTLSError(err error) string {
	if err == nil {
		return SigUnknown
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "sslv2"):
		return SigSSLv2
	case strings.Contains(msg, "first record does not look like a tls"):
		return SigTLSBadRecord
	case strings.Contains(msg, "no cipher suite"):
		return SigTLSCipherMismatch
	case strings.Contains(msg, "unsupported versions"), strings.Contains(msg, "client offered only unsupported"):
		return SigTLSOldVersion
	case strings.Contains(msg, "connection reset"):
		return SigConnectionReset
	case strings.Contains(msg, "unexpected eof"), strings.Contains(msg, "eof"):
		return SigUnexpectedEOF
	default:
		return SigTLSError
	}
}

// ClassifyFrameLength 将非法帧长（大端 4 字节）映射为协议特征。
func ClassifyFrameLength(n int) string {
	if n <= 0 {
		return SigFrameInvalid
	}
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(n))
	s := string(b[:])
	switch {
	case strings.HasPrefix(s, "GET "), s == "GET ":
		return SigHTTPGet
	case strings.HasPrefix(s, "POST"), strings.HasPrefix(s, "HEAD"), strings.HasPrefix(s, "OPTI"):
		return SigHTTPMethod
	case s == "AMQP":
		return SigAMQP
	case s == "JRMI":
		return SigJRMI
	case s == "GIOP":
		return SigGIOP
	case s == "CONN":
		return SigConnProbe
	case s == "HELP":
		return SigHelpProbe
	case b[0] == 0x16 && b[1] == 0x03:
		return SigNestedTLS
	case b[0] == 0x0d && b[1] == 0x0a:
		return SigHTTPBlank
	default:
		return SigFrameInvalid
	}
}
