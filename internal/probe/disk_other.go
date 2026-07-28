//go:build !linux && !windows

package probe

import (
	"os"
	"path/filepath"
)

func IdentifyDisk(path string) (DiskIdentity, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return DiskIdentity{}, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return DiskIdentity{}, err
	}
	return DiskIdentity{
		Path: resolved, IDs: []string{path, resolved, filepath.Base(path)},
		Size: info.Size(), SectorSize: 512,
	}, nil
}
