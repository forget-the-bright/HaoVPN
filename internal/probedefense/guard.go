// Package probedefense 识别公网对隧道口的扫描探针，记录安全事件并可自动/手动封禁源 IP。
//
// 由 serverapp 注入 transport Accept 与拒绝路径；不依赖 api 包。
// 特征/阶段/动作中文含义见 labels.go，与 docs/security-hardening.md 对照表保持一致。
package probedefense

import (
	"encoding/binary"
	"encoding/json"
	"strings"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/netutil"
	"haovpn/internal/persist"
	"haovpn/internal/timeutil"
)

// 阶段与动作常量（写入 security_events）。
const (
	PhaseTCPAccept = "tcp_accept"
	PhaseTLS       = "tls"
	PhaseFrame     = "frame"
	PhaseHandshake = "handshake"
	PhaseBanHit    = "ban_hit"

	ActionRejected     = "rejected"
	ActionBannedHit    = "banned_hit"
	ActionAutoBanned   = "auto_banned"
	ActionManualBanned = "manual_banned"
)

// Config 探针防御运行时配置（来自 security.probe_defense）。
type Config struct {
	Enabled                bool
	RecordEvents           bool
	AutoBan                bool
	BanAfterEvents         int
	BanWindowSec           int
	BanDurationSec         int
	EventRetentionDays     int
	IgnoreSignaturesForBan []string
	AllowedSourceIPs       []string // tunnel_allowed_source_ips；命中则永不自动封
}

// DefaultConfig 温和默认：启用记录与自动封禁（与 ApplyDefaults 数值对齐）。
func DefaultConfig() Config {
	return Config{
		Enabled:            true,
		RecordEvents:       true,
		AutoBan:            true,
		BanAfterEvents:     8,
		BanWindowSec:       600,
		BanDurationSec:     3600,
		EventRetentionDays: 30,
		IgnoreSignaturesForBan: []string{
			"connection_reset",
			"unexpected_eof",
			"auth_failed", // 密码错误走登录锁定，不参与探针自动封禁
		},
	}
}

// Guard 探针防御门禁：封禁检查、事件记录、自动封禁。
//
// 线程安全：方法可并发调用；内部依赖 Store 串行写。
// Enabled 只管自动记录/自动封；ip_blocks 生效查询（IsBlocked）不依赖 Enabled。
type Guard struct {
	store *persist.Store
	cfg   Config
}

// New 创建 Guard；store 不可为 nil；cfg 由调用方 ApplyDefaults 后传入。
func New(store *persist.Store, cfg Config) *Guard {
	return &Guard{store: store, cfg: cfg}
}

// Enabled 自动防御总开关（记录/自动封）；封禁表命中仍由 IsBlocked 强制拒绝。
func (g *Guard) Enabled() bool {
	if g == nil {
		return false
	}
	return g.cfg.Enabled
}

// IsBlocked 查询 IP 是否处于生效封禁（不依赖 Enabled，手动封禁始终生效）。
func (g *Guard) IsBlocked(ip string) bool {
	if g == nil || g.store == nil || ip == "" {
		return false
	}
	b, err := g.store.GetActiveIPBlock(ip)
	return err == nil && b != nil
}

// AllowSourceIP 若配置了 tunnel_allowed_source_ips，则仅白名单内允许；空列表表示不限制。
func (g *Guard) AllowSourceIP(ip string) bool {
	if g == nil || len(g.cfg.AllowedSourceIPs) == 0 {
		return true
	}
	parsed, err := netutil.ParseHostIP(ip)
	if err != nil {
		return false
	}
	return netutil.IPMatchesRules(parsed, g.cfg.AllowedSourceIPs)
}

// IsAllowlisted 是否在源白名单内（白名单 IP 永不自动封禁）。
//
// 未配置白名单时返回 false（表示「没有豁免身份」）。
func (g *Guard) IsAllowlisted(ip string) bool {
	if g == nil || len(g.cfg.AllowedSourceIPs) == 0 {
		return false
	}
	return g.AllowSourceIP(ip)
}

// AllowAccept 实现 transport.ProbeObserver：封禁或源白名单拒绝时返回 false。
//
// 已封禁 IP 始终拒绝（不看 Enabled）；源白名单与事件记录仅在 Enabled 时生效。
func (g *Guard) AllowAccept(remoteAddr string) bool {
	if g == nil {
		return true
	}
	ip, port := netutil.SplitRemoteAddr(remoteAddr)
	if g.IsBlocked(ip) {
		g.RecordBanHit(ip, port)
		logger.Warn("探针防御拒绝(已封禁) ip=%s port=%s", ip, port)
		return false
	}
	if !g.cfg.Enabled {
		return true
	}
	// 源白名单与 tunnel.CheckTunnelSourceIP 共用 netutil.IPMatchesRules，避免两套匹配语义漂移。
	if len(g.cfg.AllowedSourceIPs) > 0 && !g.AllowSourceIP(ip) {
		g.RecordReject(ip, port, PhaseTCPAccept, "source_deny", "不在 tunnel_allowed_source_ips")
		logger.Warn("探针防御拒绝(源白名单) ip=%s port=%s", ip, port)
		return false
	}
	return true
}

// OnTransportReadError 实现 transport.ProbeObserver。
//
// 读超时/已关闭连接忽略；真 TLS 协议错记事件并打一条 Warn（transport 侧不再重复 Warn）。
func (g *Guard) OnTransportReadError(remoteAddr string, err error) {
	if g == nil || !g.cfg.Enabled || err == nil || IsIgnorableTransportError(err) {
		return
	}
	ip, port := netutil.SplitRemoteAddr(remoteAddr)
	sig := ClassifyTLSError(err)
	logger.Warn("探针特征拒绝 phase=tls ip=%s port=%s signature=%s detail=%v", ip, port, sig, err)
	g.RecordReject(ip, port, PhaseTLS, sig, err.Error())
}

// OnFrameDecodeError 实现 transport.ProbeObserver。
func (g *Guard) OnFrameDecodeError(remoteAddr string, invalidLen int, err error) {
	if g == nil || !g.cfg.Enabled {
		return
	}
	ip, port := netutil.SplitRemoteAddr(remoteAddr)
	sig := ClassifyFrameLength(invalidLen)
	logger.Warn("探针特征拒绝 phase=frame ip=%s port=%s signature=%s len=%d detail=%v", ip, port, sig, invalidLen, err)
	g.RecordReject(ip, port, PhaseFrame, sig, err.Error())
}

// RecordBanHit 封禁 IP 再次连接：记事件并 hits++。
//
// 封禁命中始终记（便于审计）；RecordEvents=false 时 record 内部跳过写库。
func (g *Guard) RecordBanHit(ip, port string) {
	if g == nil || g.store == nil {
		return
	}
	if err := g.store.IncrementIPBlockHit(ip); err != nil {
		logger.Warn("封禁命中计数失败 ip=%s: %v", ip, err)
	}
	g.record(ip, port, PhaseBanHit, "banned", ActionBannedHit, "")
}

// RecordReject 记录一次拒绝并可能触发自动封禁。
func (g *Guard) RecordReject(ip, port, phase, signature, detail string) {
	if g == nil || !g.cfg.Enabled {
		return
	}
	g.record(ip, port, phase, signature, ActionRejected, detail)
	g.maybeAutoBan(ip, signature)
}

// ManualBan 管理员手动封禁（不依赖 Enabled；封禁表始终可写）。
func (g *Guard) ManualBan(ip, reason string) error {
	if g == nil || g.store == nil {
		return nil
	}
	b := persist.IPBlock{
		IP: ip, Reason: reason, Source: "manual", Enabled: true,
	}
	if g.cfg.BanDurationSec > 0 {
		exp := time.Now().Add(timeutil.Seconds(g.cfg.BanDurationSec))
		b.ExpiresAt = &exp
	}
	if err := g.store.UpsertIPBlock(b); err != nil {
		return err
	}
	g.record(ip, "", PhaseTCPAccept, "manual", ActionManualBanned, reason)
	return nil
}

// Unban 解封。
func (g *Guard) Unban(ip string) error {
	if g == nil || g.store == nil {
		return nil
	}
	return g.store.DisableIPBlock(ip)
}

func (g *Guard) record(ip, port, phase, signature, action, detail string) {
	if !g.cfg.RecordEvents {
		return
	}
	detailJSON := ""
	if detail != "" {
		b, _ := json.Marshal(map[string]string{"detail": detail})
		detailJSON = string(b)
	}
	if err := g.store.InsertSecurityEvent(persist.SecurityEvent{
		ClientIP: ip, ClientPort: port, Phase: phase,
		Signature: signature, Action: action, DetailJSON: detailJSON,
	}); err != nil {
		logger.Warn("写入 security_events 失败: %v", err)
	}
}

func (g *Guard) maybeAutoBan(ip, signature string) {
	if !g.cfg.AutoBan || ip == "" {
		return
	}
	if g.IsAllowlisted(ip) {
		return
	}
	for _, ign := range g.cfg.IgnoreSignaturesForBan {
		if signature == strings.TrimSpace(ign) {
			return
		}
	}
	window := timeutil.Seconds(g.cfg.BanWindowSec)
	if window <= 0 {
		window = 10 * time.Minute
	}
	threshold := g.cfg.BanAfterEvents
	if threshold <= 0 {
		threshold = 8
	}
	n, err := g.store.CountSecurityEventsSince(ip, time.Now().Add(-window), g.cfg.IgnoreSignaturesForBan)
	if err != nil || n < threshold {
		return
	}
	reason := "auto: " + signature + " 窗口内达阈值"
	b := persist.IPBlock{
		IP: ip, Reason: reason, Source: "auto", Signature: signature, Enabled: true,
	}
	if g.cfg.BanDurationSec > 0 {
		exp := time.Now().Add(timeutil.Seconds(g.cfg.BanDurationSec))
		b.ExpiresAt = &exp
	}
	if err := g.store.UpsertIPBlock(b); err != nil {
		logger.Warn("自动封禁失败 ip=%s: %v", ip, err)
		return
	}
	g.record(ip, "", PhaseTCPAccept, signature, ActionAutoBanned, reason)
	logger.Warn("探针防御自动封禁 ip=%s reason=%s events=%d", ip, reason, n)
}

// ClassifyTLSError 将 TLS/读错误映射为 signature。
func ClassifyTLSError(err error) string {
	if err == nil {
		return "unknown"
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "sslv2"):
		return "sslv2"
	case strings.Contains(msg, "first record does not look like a tls"):
		return "tls_bad_record"
	case strings.Contains(msg, "no cipher suite"):
		return "tls_cipher_mismatch"
	case strings.Contains(msg, "unsupported versions"), strings.Contains(msg, "client offered only unsupported"):
		return "tls_old_version"
	case strings.Contains(msg, "connection reset"):
		return "connection_reset"
	case strings.Contains(msg, "unexpected eof"), strings.Contains(msg, "eof"):
		return "unexpected_eof"
	default:
		return "tls_error"
	}
}

// ClassifyFrameLength 将非法帧长（大端 4 字节）映射为协议特征。
func ClassifyFrameLength(n int) string {
	if n <= 0 {
		return "frame_invalid"
	}
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(n))
	s := string(b[:])
	switch {
	case strings.HasPrefix(s, "GET "), s == "GET ":
		return "http_get"
	case strings.HasPrefix(s, "POST"), strings.HasPrefix(s, "HEAD"), strings.HasPrefix(s, "OPTI"):
		return "http_method"
	case s == "AMQP":
		return "amqp"
	case s == "JRMI":
		return "jrmi"
	case s == "GIOP":
		return "giop"
	case s == "CONN":
		return "conn_probe"
	case s == "HELP":
		return "help_probe"
	case b[0] == 0x16 && b[1] == 0x03:
		return "nested_tls"
	case b[0] == 0x0d && b[1] == 0x0a:
		return "http_blank"
	default:
		return "frame_invalid"
	}
}
