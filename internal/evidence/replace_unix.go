//go:build !windows

package evidence

import "os"

func replaceFile(oldPath, newPath string) error { return os.Rename(oldPath, newPath) }
