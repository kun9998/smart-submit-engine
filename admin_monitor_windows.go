//go:build windows

package main

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modKernel32              = windows.NewLazySystemDLL("kernel32.dll")
	modPsapi                 = windows.NewLazySystemDLL("psapi.dll")
	procGetSystemTimes       = modKernel32.NewProc("GetSystemTimes")
	procGlobalMemoryStatusEx = modKernel32.NewProc("GlobalMemoryStatusEx")
	procGetProcessMemoryInfo = modPsapi.NewProc("GetProcessMemoryInfo")
)

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

type processMemoryCounters struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

func readProcessRSS() (uint64, bool) {
	var pmc processMemoryCounters
	pmc.CB = uint32(unsafe.Sizeof(pmc))
	h := windows.CurrentProcess()
	r, _, err := procGetProcessMemoryInfo.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&pmc)),
		uintptr(pmc.CB),
	)
	if r == 0 {
		_ = err
		return 0, false
	}
	return uint64(pmc.WorkingSetSize), true
}

func monitorCPUStats() (usage float64, load5, load15 *float64) {
	idle1, kernel1, user1, ok1 := readWindowsCPUSample()
	if !ok1 {
		return 0, nil, nil
	}
	time.Sleep(200 * time.Millisecond)
	idle2, kernel2, user2, ok2 := readWindowsCPUSample()
	if !ok2 {
		return 0, nil, nil
	}
	idleDelta := float64(idle2 - idle1)
	kernelDelta := float64(kernel2 - kernel1)
	userDelta := float64(user2 - user1)
	totalDelta := kernelDelta + userDelta
	if totalDelta <= 0 {
		return 0, nil, nil
	}
	usage = (1 - idleDelta/totalDelta) * 100
	if usage < 0 {
		usage = 0
	}
	if usage > 100 {
		usage = 100
	}
	return usage, nil, nil
}

func monitorSystemMemory() (total, used, free uint64) {
	var ms memoryStatusEx
	ms.Length = uint32(unsafe.Sizeof(ms))
	r, _, err := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&ms)))
	if r == 0 {
		_ = err
		return 0, 0, 0
	}
	total = ms.TotalPhys
	free = ms.AvailPhys
	if total > free {
		used = total - free
	}
	return total, used, free
}

func readWindowsCPUSample() (idle, kernel, user uint64, ok bool) {
	var idleTime, kernelTime, userTime windows.Filetime
	r, _, err := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idleTime)),
		uintptr(unsafe.Pointer(&kernelTime)),
		uintptr(unsafe.Pointer(&userTime)),
	)
	if r == 0 {
		_ = err
		return 0, 0, 0, false
	}
	return filetimeToUint64(&idleTime), filetimeToUint64(&kernelTime), filetimeToUint64(&userTime), true
}

func filetimeToUint64(ft *windows.Filetime) uint64 {
	return uint64(ft.HighDateTime)<<32 + uint64(ft.LowDateTime)
}
