package source

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

type boundedBuffer struct {
	b   bytes.Buffer
	max int
}

func newBoundedBuffer(max int) *boundedBuffer { return &boundedBuffer{max: max} }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	original := len(p)
	if b.b.Len() < b.max {
		remaining := b.max - b.b.Len()
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.b.Write(p)
	}
	return original, nil
}

func (b *boundedBuffer) String() string { return b.b.String() }

type Handle struct {
	Reader   io.Reader
	Provider string
	Close    func() error
}

func Open(ctx context.Context, kind, provider, path, toolPath string, offset, size, sectorSize int64) (*Handle, error) {
	switch strings.ToLower(kind) {
	case "disk":
		if path == "" {
			return nil, errors.New("disk source path is required")
		}
		return openDisk(ctx, provider, path, toolPath, offset, size, sectorSize)
	case "ram":
		if offset != 0 {
			return nil, errors.New("RAM acquisition cannot resume from a non-zero offset")
		}
		return openRAM(ctx, provider, path, toolPath)
	default:
		return nil, fmt.Errorf("unsupported source kind %q", kind)
	}
}

func openExternalDisk(ctx context.Context, toolPath, path string, offset, size, sectorSize int64) (*Handle, error) {
	if toolPath == "" {
		return nil, errors.New("external disk streaming provider requires a verified tool_path")
	}
	cmd := exec.CommandContext(ctx, toolPath,
		"--source", path,
		"--offset", strconv.FormatInt(offset, 10),
		"--size", strconv.FormatInt(size, 10),
		"--sector-size", strconv.FormatInt(sectorSize, 10),
	)
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
		Reader: out, Provider: "signed-external-streaming-adapter",
		Close: func() error {
			if err := cmd.Wait(); err != nil {
				return fmt.Errorf("external disk streaming adapter failed: %w: %s", err, strings.TrimSpace(stderr.String()))
			}
			return nil
		},
	}, nil
}
