//go:build windows

package netstack

import (
	"fmt"
	"sync"
	"unsafe"

	"haovpn/internal/logger"

	"golang.org/x/sys/windows"
)

// 本产品 WFP 子层 GUID（固定，便于识别与清理）。
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

// KillSwitchSupported Windows 支持杀开关。
func KillSwitchSupported() error { return nil }

// EnableKillSwitch 用 WFP 阻断 AllowedIPs 出站连接。
func EnableKillSwitch(prefixes []string) error {
	prefixes = NormalizeKillPrefixes(prefixes)
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

// DisableKillSwitch 删除过滤器（连接成功后允许 AllowedIPs）。
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

// RemoveKillSwitchRules 拆除全部杀开关并关闭引擎。
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

func installFiltersLocked(prefixes []string) ([]uint64, error) {
	var ids []uint64
	r, _, _ := procFwpmTransactionBegin0.Call(uintptr(killEngine), 0)
	if r != 0 {
		return nil, fmt.Errorf("FwpmTransactionBegin0: 0x%x", r)
	}
	committed := false
	defer func() {
		if !committed {
			procFwpmTransactionAbort0.Call(uintptr(killEngine))
		}
	}()
	for i, p := range prefixes {
		id, err := addBlockFilter(p, i)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	r, _, _ = procFwpmTransactionCommit0.Call(uintptr(killEngine))
	if r != 0 {
		return nil, fmt.Errorf("FwpmTransactionCommit0: 0x%x", r)
	}
	committed = true
	return ids, nil
}

func addBlockFilter(cidr string, idx int) (uint64, error) {
	addr, mask, err := ParseCIDRToV4Mask(cidr)
	if err != nil {
		return 0, err
	}
	am := fwpV4AddrAndMask{addr: addr, mask: mask}
	cond := fwpmFilterCondition0{
		fieldKey:  condIPRemoteAddress,
		matchType: 0,
		conditionValue: fwpConditionValue0{
			typ: fwpV4AddrMask,
			val: uintptr(unsafe.Pointer(&am)),
		},
	}
	name, _ := windows.UTF16PtrFromString(fmt.Sprintf("HaoVPN-Kill-%d", idx))
	weight := uint64(0x8000000000000000)
	f := fwpmFilter0{
		displayData:         fwpmDisplayData0{name: name},
		flags:               fwpmFilterFlagClearActionRight,
		layerKey:            layerALEAuthConnectV4,
		subLayerKey:         HaoVPNSublayerGUID,
		weight:              fwpValue0{typ: fwpUint64, val: uintptr(unsafe.Pointer(&weight))},
		numFilterConditions: 1,
		filterCondition:     &cond,
		action:              fwpmAction0{typ: fwpActionBlock},
	}
	var filterID uint64
	r, _, e := procFwpmFilterAdd0.Call(
		uintptr(killEngine),
		uintptr(unsafe.Pointer(&f)),
		0,
		uintptr(unsafe.Pointer(&filterID)),
	)
	if r != 0 {
		return 0, fmt.Errorf("FwpmFilterAdd0 %s: 0x%x (%v)", cidr, r, e)
	}
	return filterID, nil
}

func removeFiltersLocked() error {
	return removeAllProductFiltersLocked()
}

// removeAllProductFiltersLocked 按子层 GUID 枚举并删除本产品全部过滤器（含进程外残留）。
func removeAllProductFiltersLocked() error {
	if killEngine == 0 {
		killFilterIDs = nil
		return nil
	}
	product := HaoVPNKillSublayerBytes()
	refs, err := enumLayerFiltersLocked()
	if err != nil {
		// 枚举失败时仍尽力删内存中记录的 ID
		var first error
		for _, id := range killFilterIDs {
			r, _, e := procFwpmFilterDeleteById0.Call(uintptr(killEngine), uintptr(id))
			if r != 0 && first == nil {
				first = fmt.Errorf("FwpmFilterDeleteById0 %d: 0x%x (%v)", id, r, e)
			}
		}
		killFilterIDs = nil
		if first != nil {
			return first
		}
		return err
	}
	ids := SelectProductFilterIDs(refs, product)
	var first error
	for _, id := range ids {
		r, _, e := procFwpmFilterDeleteById0.Call(uintptr(killEngine), uintptr(id))
		if r != 0 && first == nil {
			first = fmt.Errorf("FwpmFilterDeleteById0 %d: 0x%x (%v)", id, r, e)
		}
	}
	killFilterIDs = nil
	if len(ids) > 0 {
		logger.Info("killswitch cleaned product filters count=%d", len(ids))
	}
	return first
}

func guidToBytes(g windows.GUID) [16]byte {
	var b [16]byte
	b[0] = byte(g.Data1)
	b[1] = byte(g.Data1 >> 8)
	b[2] = byte(g.Data1 >> 16)
	b[3] = byte(g.Data1 >> 24)
	b[4] = byte(g.Data2)
	b[5] = byte(g.Data2 >> 8)
	b[6] = byte(g.Data3)
	b[7] = byte(g.Data3 >> 8)
	copy(b[8:], g.Data4[:])
	return b
}

func enumLayerFiltersLocked() ([]WFPFilterRef, error) {
	tmpl := fwpmFilterEnumTemplate0{
		layerKey:   layerALEAuthConnectV4,
		enumType:   0, // FWP_FILTER_ENUM_FULLY_CONTAINED
		actionMask: 0xffffffff,
	}
	var enumHandle windows.Handle
	r, _, e := procFwpmFilterCreateEnumHandle0.Call(
		uintptr(killEngine),
		uintptr(unsafe.Pointer(&tmpl)),
		uintptr(unsafe.Pointer(&enumHandle)),
	)
	if r != 0 {
		return nil, fmt.Errorf("FwpmFilterCreateEnumHandle0: 0x%x (%v)", r, e)
	}
	defer procFwpmFilterDestroyEnumHandle0.Call(uintptr(killEngine), uintptr(enumHandle))

	var out []WFPFilterRef
	for {
		var entries **fwpmFilter0
		var n uint32
		r, _, e = procFwpmFilterEnum0.Call(
			uintptr(killEngine),
			uintptr(enumHandle),
			100,
			uintptr(unsafe.Pointer(&entries)),
			uintptr(unsafe.Pointer(&n)),
		)
		if r != 0 {
			return out, fmt.Errorf("FwpmFilterEnum0: 0x%x (%v)", r, e)
		}
		if n == 0 || entries == nil {
			break
		}
		ptrs := unsafe.Slice(entries, n)
		for _, p := range ptrs {
			if p == nil {
				continue
			}
			out = append(out, WFPFilterRef{
				ID:       p.filterId,
				Sublayer: guidToBytes(p.subLayerKey),
			})
		}
		procFwpmFreeMemory0.Call(uintptr(unsafe.Pointer(entries)))
		if n < 100 {
			break
		}
	}
	return out, nil
}
