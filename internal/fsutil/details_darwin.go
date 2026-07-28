//go:build darwin

package fsutil

import (
	"strings"

	"golang.org/x/sys/unix"
)

func Inspect(path string) (Details, error) {
	var s unix.Statfs_t
	if err := unix.Statfs(path, &s); err != nil {
		return Details{}, err
	}
	raw := make([]byte, 0, len(s.Fstypename))
	for _, c := range s.Fstypename {
		if c == 0 {
			break
		}
		raw = append(raw, byte(c))
	}
	return Details{
		Path: path, FileSystem: strings.ToUpper(string(raw)),
		Available: uint64(s.Bavail) * uint64(s.Bsize),
	}, nil
}
