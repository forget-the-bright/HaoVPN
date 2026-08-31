//go:build windows

package netstack

import (
	"fmt"
	"unsafe"

	"haovpn/internal/logger"
	"haovpn/internal/netutil"

	"golang.org/x/sys/windows"
)

// 本文件：WFP 过滤器安装/删除（杀开关 Block 规则）。
// 状态与引擎句柄见 killswitch_windows.go；枚举见 killswitch_wfp_enum_windows.go。

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
	addr, mask, err := netutil.ParseCIDRToV4Mask(cidr)
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

