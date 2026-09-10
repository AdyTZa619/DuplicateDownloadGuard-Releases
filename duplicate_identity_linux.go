package main

import (
	"fmt"
	"os"
	"syscall"
)

func hashFileIdentityV90(_ *os.File, info os.FileInfo) string {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return fmt.Sprintf("%d:%d:%d:%d", st.Dev, st.Ino, st.Ctim.Sec, st.Ctim.Nsec)
	}
	return ""
}
