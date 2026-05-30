//go:build !linux

package fsutil

func GetInode(path string) (uint64, error) {
	return 0, nil
}
