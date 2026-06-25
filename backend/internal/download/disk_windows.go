//go:build windows

package download

import "golang.org/x/sys/windows"

func freeDiskBytes(path string) (uint64, error) {
	path16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var freeBytes uint64
	if err := windows.GetDiskFreeSpaceEx(path16, &freeBytes, nil, nil); err != nil {
		return 0, err
	}
	return freeBytes, nil
}
