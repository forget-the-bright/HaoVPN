//go:build windows

package netstack

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// 本文件：按层枚举 WFP 过滤器，筛出本产品子层（崩溃残留清理）。

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
