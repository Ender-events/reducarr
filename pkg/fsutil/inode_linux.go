//go:build linux

package fsutil

import (
	"os"
	"syscall"
)

func GetInode(path string) (uint64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, nil
	}
	return stat.Ino, nil
}
