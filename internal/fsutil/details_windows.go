//go:build windows

package fsutil

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func Inspect(path string) (Details, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Details{}, err
	}
	pathPtr, err := windows.UTF16PtrFromString(abs)
	if err != nil {
		return Details{}, err
	}
	volumePath := make([]uint16, windows.MAX_PATH+1)
	if err := windows.GetVolumePathName(pathPtr, &volumePath[0], uint32(len(volumePath))); err != nil {
		return Details{}, err
	}
	fsName := make([]uint16, 64)
	if err := windows.GetVolumeInformation(
		&volumePath[0], nil, 0, nil, nil, nil, &fsName[0], uint32(len(fsName)),
	); err != nil {
		return Details{}, err
	}
	available, err := Available(abs)
	if err != nil {
		return Details{}, err
	}
	return Details{
		Path:       windows.UTF16ToString(volumePath),
		FileSystem: strings.ToUpper(windows.UTF16ToString(fsName)),
		Available:  available,
	}, nil
}
