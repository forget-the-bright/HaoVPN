package probedefense

// 探针特征码常量（写入 security_events.signature；与 labels.go 的 map key 同源）。

const (
	SigAccountOnline   = "account_online"
	SigAuthFailed      = "auth_failed"
	SigSourceDeny      = "source_deny"
	SigHandshakeReject = "handshake_reject"
	SigHTTPGet         = "http_get"
	SigHTTPMethod      = "http_method"
	SigHTTPBlank       = "http_blank"
	SigAMQP            = "amqp"
	SigJRMI            = "jrmi"
	SigGIOP            = "giop"
	SigConnProbe       = "conn_probe"
	SigHelpProbe       = "help_probe"
	SigNestedTLS       = "nested_tls"
	SigFrameInvalid    = "frame_invalid"
	SigSSLv2           = "sslv2"
	SigTLSBadRecord    = "tls_bad_record"
	SigTLSCipherMismatch = "tls_cipher_mismatch"
	SigTLSOldVersion   = "tls_old_version"
	SigTLSError        = "tls_error"
	SigConnectionReset = "connection_reset"
	SigUnexpectedEOF   = "unexpected_eof"
	SigBanned          = "banned"
	SigManual          = "manual"
	SigUnknown         = "unknown"
)
