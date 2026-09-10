package main

import (
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

var getFileInformationByHandleExV90 = syscall.NewLazyDLL("kernel32.dll").NewProc("GetFileInformationByHandleEx")

// FILE_BASIC_INFO includes the metadata change time, unlike the older
// BY_HANDLE_FILE_INFORMATION structure. See Microsoft's FILE_BASIC_INFO docs.
type fileBasicInfoV90 struct {
	CreationTime   int64
	LastAccessTime int64
	LastWriteTime  int64
	ChangeTime     int64
	FileAttributes uint32
	_              uint32
}

func hashFileIdentityV90(f *os.File, _ os.FileInfo) (string, bool) {
	var info syscall.ByHandleFileInformation
	if syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &info) != nil {
		return "", false
	}
	if getFileInformationByHandleExV90.Find() != nil {
		return "", false
	}
	var basic fileBasicInfoV90
	ok, _, _ := getFileInformationByHandleExV90.Call(f.Fd(), 0, // FileBasicInfo
		uintptr(unsafe.Pointer(&basic)), unsafe.Sizeof(basic))
	if ok == 0 || basic.ChangeTime <= 0 || basic.LastWriteTime <= 0 {
		return "", false
	}
	identity := fmt.Sprintf("win2:%d:%d:%d:%d:%d:%d", info.VolumeSerialNumber,
		info.FileIndexHigh, info.FileIndexLow, basic.CreationTime, basic.LastWriteTime, basic.ChangeTime)
	latest := basic.ChangeTime
	if basic.LastWriteTime > latest {
		latest = basic.LastWriteTime
	}
	stamp := syscall.Filetime{LowDateTime: uint32(latest), HighDateTime: uint32(uint64(latest) >> 32)}
	// Rapid writes can share a filesystem timestamp (FAT write resolution is
	// two seconds). A digest first observed in this interval must NOT be stored
	// as reusable: waiting until a later lookup would still trust stale bytes.
	// https://learn.microsoft.com/en-us/windows/win32/sysinfo/file-times
	cacheable := time.Since(time.Unix(0, stamp.Nanoseconds())) > 2*time.Second
	return identity, cacheable
}
