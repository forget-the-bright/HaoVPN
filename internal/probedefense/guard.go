// Package probedefense 识别公网对隧道口的扫描探针，记录安全事件并可自动/手动封禁源 IP。
//
// 由 serverapp 注入 transport Accept 与拒绝路径；不依赖 api 包。
// 特征/阶段/动作中文含义见 labels.go，与 docs/security-hardening.md 对照表保持一致。
package probedefense

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/netutil"
	"haovpn/internal/persist"
	"haovpn/internal/transport"
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

// BanDurationUseDefault 手动封禁未指定 duration_sec 时使用服务端 probe_defense.ban_duration_sec。
const BanDurationUseDefault = -1

// MaxBanDurationSec 手动封禁时长上限（10 年），防止误填过大数值。
const MaxBanDurationSec = 315360000

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
	BanExemptIPs           []string // probe_defense.ban_exempt_ips；启动 yaml + DB 合并
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
	exemptMu     sync.RWMutex
	banExemptIPs []string // yaml + DB 合并；ReloadBanExempt 更新
}

// New 创建 Guard；store 不可为 nil；cfg 由调用方 ApplyDefaults 后传入。
func New(store *persist.Store, cfg Config) *Guard {
	g := &Guard{store: store, cfg: cfg}
	g.banExemptIPs = append([]string(nil), cfg.BanExemptIPs...)
	return g
}

// Enabled 自动防御总开关（记录/自动封）；封禁表命中仍由 IsBlocked 强制拒绝。
func (g *Guard) Enabled() bool {
	if g == nil {
		return false
	}
	return g.cfg.Enabled
}

// IsBlocked 查询 IP 是否处于生效封禁（豁免 IP 视为未封禁）。
func (g *Guard) IsBlocked(ip string) bool {
	if g == nil || g.store == nil || ip == "" {
		return false
	}
	if g.IsBanExempt(ip) {
		return false
	}
	return g.isBlockedRaw(ip)
}

// isBlockedRaw 仅查 ip_blocks，不考虑豁免。
func (g *Guard) isBlockedRaw(ip string) bool {
	b, err := g.store.GetActiveIPBlock(ip)
	return err == nil && b != nil
}

// AllowSourceIP 若配置了 tunnel_allowed_source_ips，则仅白名单内允许；空列表表示不限制。
func (g *Guard) AllowSourceIP(ip string) bool {
	if g == nil || len(g.cfg.AllowedSourceIPs) == 0 {
		return true
	}
	return netutil.CheckSourceIPAllowed(ip, g.cfg.AllowedSourceIPs) == nil
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

// CheckAccept 实现 transport.ProbeObserver：返回是否允许接入及 TLS 前拒绝 banner。
func (g *Guard) CheckAccept(remoteAddr string) (allow bool, rejectBanner string) {
	if g == nil {
		return true, ""
	}
	ip, port := netutil.SplitRemoteAddr(remoteAddr)
	if g.isBlockedRaw(ip) && !g.IsBanExempt(ip) {
		g.RecordBanHit(ip, port)
		logger.Warn("探针防御拒绝(已封禁) ip=%s port=%s", ip, port)
		return false, transport.BannerIPBanned
	}
	if len(g.cfg.AllowedSourceIPs) > 0 && !g.AllowSourceIP(ip) {
		g.RecordReject(ip, port, PhaseTCPAccept, SigSourceDeny, "不在 tunnel_allowed_source_ips")
		logger.Warn("探针防御拒绝(源白名单) ip=%s port=%s", ip, port)
		return false, ""
	}
	return true, ""
}

// AllowAccept 实现 transport.ProbeObserver（兼容旧调用路径）。
func (g *Guard) AllowAccept(remoteAddr string) bool {
	allow, _ := g.CheckAccept(remoteAddr)
	return allow
}

// OnTransportReadError 实现 transport.ProbeObserver。
//
// 读超时/已关闭连接忽略；真 TLS 协议错记事件并打一条 Warn（transport 侧不再重复 Warn）。
func (g *Guard) OnTransportReadError(remoteAddr string, err error) {
	if g == nil || err == nil || IsIgnorableTransportError(err) {
		return
	}
	ip, port := netutil.SplitRemoteAddr(remoteAddr)
	sig := ClassifyTLSError(err)
	logger.Warn("探针特征拒绝 phase=tls ip=%s port=%s signature=%s detail=%v", ip, port, sig, err)
	g.RecordReject(ip, port, PhaseTLS, sig, err.Error())
}

// OnFrameDecodeError 实现 transport.ProbeObserver。
func (g *Guard) OnFrameDecodeError(remoteAddr string, invalidLen int, err error) {
	if g == nil {
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
	g.record(ip, port, PhaseBanHit, SigBanned, ActionBannedHit, "")
}

// RecordReject 记录一次探针自动路径拒绝并可能触发自动封禁。
//
// record_events 控制是否写 security_events；enabled 控制是否参与 maybeAutoBan 计数。
func (g *Guard) RecordReject(ip, port, phase, signature, detail string) {
	if g == nil {
		return
	}
	if g.cfg.RecordEvents {
		g.record(ip, port, phase, signature, ActionRejected, detail)
	}
	if g.cfg.Enabled {
		g.maybeAutoBan(ip, signature)
	}
}

// ManualBan 管理员手动封禁（不依赖 Enabled；封禁表始终可写）。
//
// ip 须为可解析的 IP 地址（非空主机名）。
// durationSec：BanDurationUseDefault(-1) 使用 cfg.BanDurationSec；0 永久；>0 指定秒数。
func (g *Guard) ManualBan(ip, reason string, durationSec int) error {
	if g == nil || g.store == nil {
		return ErrProbeGuardNotReady
	}
	if err := ManualBanStore(g.store, g.IsBanExempt, ip, reason, durationSec, g.cfg.BanDurationSec); err != nil {
		return err
	}
	ip = strings.TrimSpace(ip)
	g.record(ip, "", PhaseTCPAccept, SigManual, ActionManualBanned, reason)
	return nil
}

// ValidateManualBanDuration 校验手动封禁时长；仅用于 API 显式传入的 duration_sec（非 UseDefault）。
func ValidateManualBanDuration(durationSec int) error {
	if durationSec == BanDurationUseDefault {
		return nil
	}
	if durationSec < 0 {
		return fmt.Errorf("duration_sec 不能为负数")
	}
	if durationSec > 0 && durationSec < 60 {
		return fmt.Errorf("duration_sec 须至少 60 秒或设为 0（永久）")
	}
	if durationSec > MaxBanDurationSec {
		return fmt.Errorf("duration_sec 不能超过 %d 秒（10 年）", MaxBanDurationSec)
	}
	return nil
}

// resolveBanExpiry 根据 durationSec 计算封禁过期时间；nil 表示永久。
func (g *Guard) resolveBanExpiry(durationSec int) *time.Time {
	return resolveBanExpiryTime(durationSec, g.cfg.BanDurationSec)
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
