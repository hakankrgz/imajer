//go:build !darwin && !windows

package main

import (
	"context"
	"errors"
)

func browseNativePath(context.Context, string) (string, bool, error) {
	return "", false, errors.New("yerel dosya seçici yalnız macOS ve Windows masaüstünde destekleniyor")
}
