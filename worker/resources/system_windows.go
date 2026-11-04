package resources

import (
	"runtime"
	"unsafe"

	"transcoder/model"

	"golang.org/x/sys/windows"
)

var kernel = windows.NewLazySystemDLL("kernel32.dll")
var getSystemTimes = kernel.NewProc("GetSystemTimes")
var globalMemoryStatus = kernel.NewProc("GlobalMemoryStatusEx")
var getCurrentProcessor = kernel.NewProc("GetCurrentProcessorNumberEx")
var getProcessorGroups = kernel.NewProc("GetActiveProcessorGroupCount")

func pinCPUThread() func() {
	runtime.LockOSThread()
	return runtime.UnlockOSThread
}

type memoryStatus struct {
	Length, Load                                                       uint32
	TotalPhysical, AvailablePhysical, TotalPageFile, AvailablePageFile uint64
	TotalVirtual, AvailableVirtual, AvailableExtendedVirtual           uint64
}

func fileTime(v windows.Filetime) uint64 { return uint64(v.HighDateTime)<<32 | uint64(v.LowDateTime) }

func readSystem() systemSample {
	var sample systemSample
	var idle, kernelTime, user windows.Filetime
	if ok, _, _ := getSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernelTime)), uintptr(unsafe.Pointer(&user))); ok != 0 {
		var processor struct {
			Group            uint16
			Number, Reserved byte
		}
		_, _, _ = getCurrentProcessor.Call(uintptr(unsafe.Pointer(&processor)))
		cores := int(windows.GetActiveProcessorCount(processor.Group))
		scope := "host"
		if groups, _, _ := getProcessorGroups.Call(); groups > 1 {
			scope = "processor-group"
		}
		sample.cpu = &cpuCounters{total: fileTime(kernelTime) + fileTime(user), idle: fileTime(idle), cores: cores, scope: scope}
	}
	memory := memoryStatus{}
	memory.Length = uint32(unsafe.Sizeof(memory))
	if ok, _, _ := globalMemoryStatus.Call(uintptr(unsafe.Pointer(&memory))); ok != 0 && memory.TotalPhysical > 0 && memory.AvailablePhysical <= memory.TotalPhysical {
		sample.memory = &model.MemoryResources{TotalBytes: memory.TotalPhysical, UsedBytes: memory.TotalPhysical - memory.AvailablePhysical}
	}
	return sample
}
