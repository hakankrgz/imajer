//go:build !windows

package source

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func openDisk(ctx context.Context, provider, path, toolPath string, offset, size, sectorSize int64) (*Handle, error) {
	if sectorSize <= 0 {
		sectorSize = 512
	}
	remaining := size - offset
	if remaining < 0 || offset%sectorSize != 0 || (remaining > 0 && remaining%sectorSize != 0) {
		return nil, fmt.Errorf("offset and size must align to sector size %d", sectorSize)
	}
	selected := strings.ToLower(provider)
	if selected == "" || selected == "auto" {
		if p, err := exec.LookPath("dc3dd"); err == nil {
			return commandDisk(ctx, p, "dc3dd", path, offset, remaining, sectorSize)
		}
		if p, err := exec.LookPath("dd"); err == nil {
			return commandDisk(ctx, p, "dd", path, offset, remaining, sectorSize)
		}
		selected = "native"
	}
	switch selected {
	case "external", "adapter":
		return openExternalDisk(ctx, toolPath, path, offset, size, sectorSize)
	case "dc3dd", "dd":
		p, err := exec.LookPath(selected)
		if err != nil {
			return nil, err
		}
		return commandDisk(ctx, p, selected, path, offset, remaining, sectorSize)
	case "native", "native-readonly":
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open source read-only: %w", err)
		}
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			f.Close()
			return nil, fmt.Errorf("seek source: %w", err)
		}
		return &Handle{Reader: io.LimitReader(f, remaining), Provider: "native-readonly", Close: f.Close}, nil
	default:
		return nil, fmt.Errorf("unsupported disk provider %q", provider)
	}
}

func commandDisk(ctx context.Context, binary, provider, path string, offset, remaining, sectorSize int64) (*Handle, error) {
	skip := offset / sectorSize
	count := remaining / sectorSize
	var args []string
	if provider == "dc3dd" {
		args = []string{
			"if=" + path, "ssz=" + strconv.FormatInt(sectorSize, 10),
			"iskip=" + strconv.FormatInt(skip, 10), "cnt=" + strconv.FormatInt(count, 10),
			"bufsz=8M",
		}
	} else {
		args = []string{
			"if=" + path, "bs=" + strconv.FormatInt(sectorSize, 10),
			"skip=" + strconv.FormatInt(skip, 10), "count=" + strconv.FormatInt(count, 10),
			"iflag=fullblock", "status=none",
		}
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	stderr := newBoundedBuffer(64 << 10)
	cmd.Stderr = stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &Handle{
		Reader: out, Provider: provider,
		Close: func() error {
			if err := cmd.Wait(); err != nil {
				return fmt.Errorf("%s failed: %w: %s", provider, err, strings.TrimSpace(stderr.String()))
			}
			return nil
		},
	}, nil
}
