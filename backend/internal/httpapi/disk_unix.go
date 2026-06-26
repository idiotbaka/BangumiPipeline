//go:build !windows

package httpapi

import "syscall"

func diskSpace(path string) (diskSpaceStats, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return diskSpaceStats{}, err
	}
	return diskSpaceStats{
		FreeBytes:  uint64(stat.Bavail) * uint64(stat.Bsize),
		TotalBytes: uint64(stat.Blocks) * uint64(stat.Bsize),
	}, nil
}
