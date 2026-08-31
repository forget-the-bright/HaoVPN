//go:build windows

package netstack

import (
	"fmt"
	"sync"
	"unsafe"

	"haovpn/internal/logger"
	"haovpn/internal/netutil"

	"golang.org/x/sys/windows"
)

// HaoVPNSublayerGUID 本产品 WFP 杀开关专用子层 GUID（固定，便于枚举与崩溃后清理）。
var (
	HaoVPNSublayerGUID = windows.GUID{Data1: 0xa1b2c3d4, Data2: 0xe5f6, Data3: 0x7890, Data4: [8]byte{0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67, 0x89}}
	// FWPM_LAYER_ALE_AUTH_CONNECT_V4
	layerALEAuthConnectV4 = windows.GUID{Data1: 0xc38d57d1, Data2: 0x05a7, Data3: 0x4c33, Data4: [8]byte{0x90, 0x4f, 0x7f, 0xbc, 0xee, 0xe6, 0x0e, 0x82}}
	// FWPM_CONDITION_IP_REMOTE_ADDRESS
	condIPRemoteAddress = windows.GUID{Data1: 0xb235a9a0, Data2: 0x1d51, Data3: 0x4640, Data4: [8]byte{0xa9, 0x85, 0xa4, 0x7d, 0xa2, 0x4e, 0x7c, 0x1c}}
)

const (
	fwpUint64              = 0x9
	fwpV4AddrMask          = 0x100
	fwpActionFlagTerminate = 0x00001000
	fwpActionBlock         = 0x00000001 | fwpActionFlagTerminate
	fwpmFilterFlagClearActionRight = 0x00000008
	rpcCAuthNDefault       = 0
)

var (
	fwpuclnt                   = windows.NewLazySystemDLL("fwpuclnt.dll")
	procFwpmEngineOpen0        = fwpuclnt.NewProc("FwpmEngineOpen0")
	procFwpmEngineClose0       = fwpuclnt.NewProc("FwpmEngineClose0")
	procFwpmSubLayerAdd0       = fwpuclnt.NewProc("FwpmSubLayerAdd0")
	procFwpmFilterAdd0         = fwpuclnt.NewProc("FwpmFilterAdd0")
	procFwpmFilterDeleteById0  = fwpuclnt.NewProc("FwpmFilterDeleteById0")
	procFwpmTransactionBegin0  = fwpuclnt.NewProc("FwpmTransactionBegin0")
	procFwpmTransactionCommit0 = fwpuclnt.NewProc("FwpmTransactionCommit0")
	procFwpmTransactionAbort0  = fwpuclnt.NewProc("FwpmTransactionAbort0")
	procFwpmFilterCreateEnumHandle0  = fwpuclnt.NewProc("FwpmFilterCreateEnumHandle0")
	procFwpmFilterEnum0              = fwpuclnt.NewProc("FwpmFilterEnum0")
	procFwpmFilterDestroyEnumHandle0 = fwpuclnt.NewProc("FwpmFilterDestroyEnumHandle0")
	procFwpmFreeMemory0              = fwpuclnt.NewProc("FwpmFreeMemory0")
)

type fwpByteBlob struct {
	size uint32
	data *byte
}

type fwpmDisplayData0 struct {
	name        *uint16
	description *uint16
}

type fwpmSublayer0 struct {
	subLayerKey  windows.GUID
	displayData  fwpmDisplayData0
	flags        uint32
	providerKey  *windows.GUID
	providerData fwpByteBlob
	weight       uint16
}

type fwpValue0 struct {
	typ uint32
	val uintptr
}

type fwpmAction0 struct {
	typ        uint32
	filterType windows.GUID
}

type fwpConditionValue0 struct {
	typ uint32
	val uintptr
}

type fwpmFilterCondition0 struct {
	fieldKey       windows.GUID
	matchType      uint32
	conditionValue fwpConditionValue0
}

type fwpV4AddrAndMask struct {
	addr uint32
	mask uint32
}

type fwpmFilter0 struct {
	filterKey           windows.GUID
	displayData         fwpmDisplayData0
	flags               uint32
	providerKey         *windows.GUID
	providerData        fwpByteBlob
	layerKey            windows.GUID
	subLayerKey         windows.GUID
	weight              fwpValue0
	numFilterConditions uint32
	filterCondition     *fwpmFilterCondition0
	action              fwpmAction0
	context             uint64
	reserved            *windows.GUID
	filterId            uint64
	effectiveWeight     fwpValue0
}

type fwpmFilterEnumTemplate0 struct {
	providerKey         *windows.GUID
	layerKey            windows.GUID
	flags               uint32
	enumType            uint32
	flags2              uint32
	actionMask          uint32
	providerContextKey  *windows.GUID
	numFilterConditions uint32
	filterCondition     *fwpmFilterCondition0
	calloutKeyCount     uint32
	calloutKey          *windows.GUID
}

var (
	killMu        sync.Mutex
	killEngine    windows.Handle
	killFilterIDs []uint64
	killPrefixes  []string
)

// KillSwitchSupported 探测当前平台是否支持杀开关。
//
// 返回：Windows 恒 nil；非 Windows 由 killswitch_other 返回不支持 error。
func KillSwitchSupported() error { return nil }

// EnableKillSwitch 在 VPN 未连接时用 WFP 阻断 AllowedIPs 前缀的出站 IPv4 连接。
//
// 参数：prefixes — AllowedIPs CIDR 列表；经 netutil.DedupTrimNonEmpty 去重。
// 返回：前缀为空或 WFP 安装失败时 error。
// 副作用：打开 FWP 引擎、注册子层、添加 Block 过滤器；安装前清理本子层旧规则。
// 并发：包内 killMu 串行化。
func EnableKillSwitch(prefixes []string) error {
	prefixes = netutil.DedupTrimNonEmpty(prefixes)
	if len(prefixes) == 0 {
		return fmt.Errorf("杀开关前缀为空")
	}
	killMu.Lock()
	defer killMu.Unlock()
	if err := ensureEngineLocked(); err != nil {
		return err
	}
	// 安装前删除本产品子层全部旧过滤器（含崩溃残留）。
	_ = removeAllProductFiltersLocked()
	ids, err := installFiltersLocked(prefixes)
	if err != nil {
		return err
	}
	killFilterIDs = ids
	killPrefixes = append([]string{}, prefixes...)
	logger.Info("killswitch enabled (WFP) prefixes=%v filters=%d", prefixes, len(ids))
	return nil
}

// DisableKillSwitch 隧道握手成功后删除 Block 过滤器，允许 AllowedIPs 正常出站。
//
// 返回：WFP 删除失败时 error；引擎保持打开供后续 Enable 复用。
// 副作用：删除 HaoVPN 子层下全部过滤器。
func DisableKillSwitch() error {
	killMu.Lock()
	defer killMu.Unlock()
	if err := ensureEngineLocked(); err != nil {
		return err
	}
	if err := removeAllProductFiltersLocked(); err != nil {
		return err
	}
	logger.Info("killswitch disabled (WFP)")
	return nil
}

// RemoveKillSwitchRules 客户端退出或 Teardown 时拆除全部杀开关规则并关闭 WFP 引擎。
//
// 返回：删除过滤器过程中的首个 error（若有）；始终清空 killFilterIDs/killPrefixes。
// 副作用：FwpmEngineClose0；须在路由清理前或按 clientapp 约定顺序调用。
func RemoveKillSwitchRules() error {
	killMu.Lock()
	defer killMu.Unlock()
	var err error
	if killEngine != 0 {
		err = removeAllProductFiltersLocked()
		procFwpmEngineClose0.Call(uintptr(killEngine))
		killEngine = 0
	}
	killFilterIDs = nil
	killPrefixes = nil
	logger.Info("killswitch rules removed (WFP)")
	return err
}

func ensureEngineLocked() error {
	if killEngine != 0 {
		return nil
	}
	var eng windows.Handle
	r, _, err := procFwpmEngineOpen0.Call(0, uintptr(rpcCAuthNDefault), 0, 0, uintptr(unsafe.Pointer(&eng)))
	if r != 0 {
		return fmt.Errorf("FwpmEngineOpen0: 0x%x (%w)", r, err)
	}
	killEngine = eng
	name, _ := windows.UTF16PtrFromString("HaoVPN KillSwitch")
	desc, _ := windows.UTF16PtrFromString("Block AllowedIPs when VPN disconnected")
	sub := fwpmSublayer0{
		subLayerKey: HaoVPNSublayerGUID,
		displayData: fwpmDisplayData0{name: name, description: desc},
		weight:      0x100,
	}
	procFwpmSubLayerAdd0.Call(uintptr(killEngine), uintptr(unsafe.Pointer(&sub)), 0)
	return nil
}
