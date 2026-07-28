//go:build linux

package source

import "os"

func osOpen(path string) (*os.File, error) { return os.Open(path) }
