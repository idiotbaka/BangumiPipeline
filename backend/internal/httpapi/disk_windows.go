//go:build windows

package httpapi

import "golang.org/x/sys/windows"

func diskSpace(path string) (diskSpaceStats, error) {
	path16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return diskSpaceStats{}, err
	}
	var freeBytes uint64
	var totalBytes uint64
	if err := windows.GetDiskFreeSpaceEx(path16, &freeBytes, &totalBytes, nil); err != nil {
		return diskSpaceStats{}, err
	}
	return diskSpaceStats{FreeBytes: freeBytes, TotalBytes: totalBytes}, nil
}
