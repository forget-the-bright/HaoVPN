//go:build windows

package tun

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wintun"

	"haovpn/internal/brand"
	"haovpn/internal/logger"
	"haovpn/internal/tun/wintundll"
	"haovpn/internal/winnet"
)

// wintunOpenMu 串行化 Open/Create/预热，避免 GUI 预热与登录 Open 并发 Create。
var wintunOpenMu sync.Mutex

// warmedAdapter 预热持有的适配器句柄（Create 后禁止 Close，否则会从系统卸掉）。
// 正式 Open 经 takeWarmedAdapterLocked 接管；仅二次预热替换或进程退出时才 Close。
var (
	warmedName    string
	warmedAdapter *wintun.Adapter
)

// haovpnWintunGUID 固定 GUID，使同 WintunPool 适配器在 Windows 上身份稳定，减少重命名孤儿网卡。
var haovpnWintunGUID = windows.GUID{
	Data1: 0x8A4F2C1E,
	Data2: 0x5B3D,
	Data3: 0x4E9A,
	Data4: [8]byte{0x9F, 0x2A, 0x7C, 0x10, 0x48, 0x56, 0x50, 0x4E},
}

// prepareWintunAdapter 启动前清理 Windows 上因重名产生的 Wintun 孤儿网卡（如 haovpn0 1）。
//
// 参数：configName — yaml tun.name，须与 Wintun OpenAdapter 名一致。
// 返回：PowerShell 执行失败时 error；无孤儿或清理成功时 nil。
// 优化：先 HasWintunOrphanAdapters（GAA）；无孤儿则跳过冷启 PS（公司机可省数秒）。
// 脚本唯一源：winnet.BuildPrepareWintunOrphanScript（禁止本包再维护第二套 PS）。
func prepareWintunAdapter(configName string) error {
	if configName == "" {
		return nil
	}
	start := time.Now()
	if !winnet.HasWintunOrphanAdapters(configName) {
		logger.Info("tun_open stage=prepare_orphan skipped reason=no_orphan elapsed=%s name=%s", time.Since(start), configName)
		return nil
	}
	ps := winnet.BuildPrepareWintunOrphanScript(configName)
	out, err := winnet.RunPSOneShot(ps)
	trimmed := strings.TrimSpace(string(out))
	if trimmed != "" {
		logger.Info("Wintun 启动前清理: %s", trimmed)
	}
	logger.Info("tun_open stage=prepare_orphan elapsed=%s name=%s ran_ps=true", time.Since(start), configName)
	if err != nil {
		return fmt.Errorf("清理 Wintun 孤儿网卡: %w", err)
	}
	return nil
}

// takeWarmedAdapterLocked 若预热槽位匹配 name，移交句柄并清空槽位。调用方须已持 wintunOpenMu。
func takeWarmedAdapterLocked(name string) *wintun.Adapter {
	if warmedAdapter == nil || warmedName != name {
		return nil
	}
	ad := warmedAdapter
	warmedAdapter = nil
	warmedName = ""
	return ad
}

// releaseWarmedLocked 丢弃预热槽位（仅 Close Create 得到的句柄会卸适配器——仅在替换/退出时调用）。
func releaseWarmedLocked() {
	if warmedAdapter == nil {
		return
	}
	// Create 路径留下的句柄 Close 会卸系统适配器；此处仅在将被新预热替换时调用。
	// 若适配器已由 Open 接管则槽位已空。为避免误卸仍在用的系统适配器：
	// 预热持有期间尚未 StartSession，Close 会卸掉——替换预热前应先尝试仅放弃句柄。
	// 按 Wintun：Close Create 的 Adapter 会 Remove；故二次预热同名时应先 Open 探测而非 Close。
	warmedAdapter.Close()
	warmedAdapter = nil
	warmedName = ""
}

// openWintunAdapter 优先接管预热句柄 / Open 已有适配器；失败则清理孤儿后 Create。
func openWintunAdapter(name string) (*wintun.Adapter, bool, error) {
	wintunOpenMu.Lock()
	defer wintunOpenMu.Unlock()
	return openWintunAdapterLocked(name)
}

// openWintunAdapterLocked 调用方须已持 wintunOpenMu。
func openWintunAdapterLocked(name string) (*wintun.Adapter, bool, error) {
	installWintunLogger()
	stageStart := time.Now()

	if ad := takeWarmedAdapterLocked(name); ad != nil {
		logger.Info("tun_open stage=reuse from_warmup elapsed=%s name=%s", time.Since(stageStart), name)
		return ad, true, nil
	}

	adapter, err := wintun.OpenAdapter(name)
	if err == nil {
		logger.Info("tun_open stage=reuse elapsed=%s name=%s", time.Since(stageStart), name)
		logger.Debug("Wintun OpenAdapter 成功: %s", name)
		return adapter, true, nil
	}
	openErr := err
	logger.Debug("Wintun OpenAdapter 未命中 %s: %v，尝试清理后 Create", name, err)

	if err := prepareWintunAdapter(name); err != nil {
		logger.Warn("Wintun 孤儿网卡清理失败（继续 Create）: %v", err)
	}

	adapter, err = wintun.OpenAdapter(name)
	if err == nil {
		logger.Info("tun_open stage=reuse elapsed=%s name=%s after_orphan_cleanup=true", time.Since(stageStart), name)
		logger.Info("Wintun 清理后复用适配器: %s", name)
		return adapter, true, nil
	}

	createStart := time.Now()
	adapter, err = wintun.CreateAdapter(name, brand.WintunPool, &haovpnWintunGUID)
	if err != nil {
		logger.Warn("tun_open stage=create fail elapsed=%s name=%s open_err=%v create_err=%v",
			time.Since(stageStart), name, openErr, err)
		return nil, false, fmt.Errorf("wintun open/create: %w", err)
	}
	logger.Info("tun_open stage=create elapsed=%s name=%s open_miss=%v", time.Since(createStart), name, openErr)
	logger.Debug("Wintun CreateAdapter 新建: %s", name)
	return adapter, false, nil
}

// warmupPlatform 预创建/打开适配器并持有句柄（禁止 Close，否则 Create 适配器会被卸掉）。
// 正式 Open 经 takeWarmedAdapterLocked 接管；登录应看到 reuse from_warmup。
func warmupPlatform(name string) error {
	start := time.Now()
	ensureStart := time.Now()
	if err := wintundll.Ensure(); err != nil {
		return err
	}
	logger.Info("tun_warmup stage=ensure_dll elapsed=%s", time.Since(ensureStart))
	if name == "" {
		name = brand.DefaultTunName
	}
	wintunOpenMu.Lock()
	defer wintunOpenMu.Unlock()

	// 已预热同名：保持句柄，勿重复 Create/Close
	if warmedAdapter != nil && warmedName == name {
		logger.Info("tun_warmup stage=reuse name=%s elapsed=%s held=true already=true", name, time.Since(start))
		return nil
	}
	// 不同名残留：仅丢弃槽位。Close Create 会卸适配器——若旧名仍在系统，优先 Open 探测。
	if warmedAdapter != nil {
		old := warmedName
		releaseWarmedLocked()
		logger.Warn("tun_warmup 替换预热槽位 old=%s new=%s（旧句柄已 Close）", old, name)
	}

	openStart := time.Now()
	adapter, reused, err := openWintunAdapterLocked(name)
	if err != nil {
		return err
	}
	logger.Info("tun_warmup stage=create_or_open elapsed=%s reused=%v", time.Since(openStart), reused)
	winnet.RegisterFromLUID(name, adapter.LUID())
	// 新建适配器时在预热阶段禁用 IPv6，避免正式 Open reused=true 跳过 disable_v6。
	if !reused {
		v6Start := time.Now()
		if err := winnet.DisableInterfaceIPv6(name); err != nil {
			logger.Debug("tun_warmup disable_v6 fail name=%s elapsed=%s: %v", name, time.Since(v6Start), err)
		} else {
			logger.Info("tun_warmup stage=disable_v6 elapsed=%s name=%s", time.Since(v6Start), name)
		}
	}
	warmedName = name
	warmedAdapter = adapter
	stage := "create"
	if reused {
		stage = "reuse"
	}
	logger.Info("tun_warmup stage=%s name=%s elapsed=%s held=true", stage, name, time.Since(start))
	return nil
}
