package main

import (
	"fmt"
	"os"
	"syscall"
)

func hashFileIdentityV90(f *os.File, _ os.FileInfo) string {
	var info syscall.ByHandleFileInformation
	if syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &info) != nil {
		return ""
	}
	return fmt.Sprintf("%d:%d:%d:%d:%d", info.VolumeSerialNumber, info.FileIndexHigh,
		info.FileIndexLow, info.CreationTime.HighDateTime, info.CreationTime.LowDateTime)
}
