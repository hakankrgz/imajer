//go:build windows

package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

func openDisk(ctx context.Context, provider, path, toolPath string, offset, size, sectorSize int64) (*Handle, error) {
	provider = strings.ToLower(provider)
	if provider == "external" || provider == "adapter" {
		return openExternalDisk(ctx, toolPath, path, offset, size, sectorSize)
	}
	if provider != "" && provider != "auto" && provider != "native" && provider != "native-readonly" {
		if provider == "ftk" || provider == "ftkimager" {
			return nil, errors.New("FTK's file-destination acquisition is disabled in zero-image-footprint mode; use native-readonly or a signed range-streaming adapter")
		}
		return nil, fmt.Errorf("Windows disk provider %q is not a registered streaming provider; use native-readonly", provider)
	}
	if sectorSize <= 0 {
		sectorSize = 512
	}
	if offset%sectorSize != 0 || size%sectorSize != 0 {
		return nil, fmt.Errorf("offset and size must align to sector size %d", sectorSize)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open physical drive read-only: %w", err)
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		f.Close()
		return nil, err
	}
	return &Handle{Reader: io.LimitReader(f, size-offset), Provider: "native-readonly", Close: f.Close}, nil
}
