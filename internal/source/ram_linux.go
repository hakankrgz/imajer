//go:build linux

package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

func openRAM(ctx context.Context, provider, path, toolPath string) (*Handle, error) {
	switch strings.ToLower(provider) {
	case "", "auto", "avml":
		if provider != "avml" && toolPath != "" &&
			strings.HasSuffix(strings.ToLower(toolPath), ".ko") {
			return openLiME(ctx, toolPath)
		}
		tool := toolPath
		if tool == "" {
			tool = "avml"
		}
		resolved, err := exec.LookPath(tool)
		if err == nil {
			return openAVML(ctx, resolved, path)
		}
		if provider == "avml" {
			return nil, fmt.Errorf("AVML not found: %w", err)
		}
		return nil, errors.New("no usable RAM provider: install signed AVML or specify a LiME module")
	case "lime":
		if toolPath == "" {
			return nil, errors.New("LiME requires tool_path pointing to an exact-kernel .ko module")
		}
		return openLiME(ctx, toolPath)
	case "direct":
		if path == "" {
			return nil, errors.New("direct RAM provider requires a readable source path")
		}
		return openDirect(path)
	default:
		return nil, fmt.Errorf("unsupported Linux RAM provider %q", provider)
	}
}

func openDirect(path string) (*Handle, error) {
	f, err := openFile(path)
	if err != nil {
		return nil, err
	}
	return &Handle{Reader: f, Provider: "direct", Close: f.Close}, nil
}

// openFile exists to keep all source files opened read-only in one auditable place.
func openFile(path string) (io.ReadCloser, error) {
	return osOpen(path)
}

func openAVML(ctx context.Context, tool, source string) (*Handle, error) {
	args := []string{"acquire"}
	if source != "" {
		args = append(args, "--source", source)
	}
	args = append(args, "/dev/stdout")
	cmd := exec.CommandContext(ctx, tool, args...)
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
		Reader: out, Provider: "avml",
		Close: func() error {
			err := cmd.Wait()
			if err != nil {
				return fmt.Errorf("AVML failed: %w: %s", err, strings.TrimSpace(stderr.String()))
			}
			return nil
		},
	}, nil
}

func openLiME(ctx context.Context, module string) (*Handle, error) {
	if filepath.Ext(module) != ".ko" {
		return nil, errors.New("LiME tool_path must end in .ko")
	}
	port, err := reservePort()
	if err != nil {
		return nil, err
	}
	arg := fmt.Sprintf("path=tcp:%d format=lime localhostonly=1 timeout=1000", port)
	cmd := exec.CommandContext(ctx, "insmod", module, arg)
	stderr := newBoundedBuffer(64 << 10)
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start LiME: %w", err)
	}
	var conn net.Conn
	for i := 0; i < 50; i++ {
		conn, err = net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 200*time.Millisecond)
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	if conn == nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("connect to LiME stream: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var once sync.Once
	closeFn := func() error {
		var result error
		once.Do(func() {
			_ = conn.Close()
			waitErr := cmd.Wait()
			removeErr := exec.Command("rmmod", "lime").Run()
			result = errors.Join(waitErr, removeErr)
		})
		return result
	}
	return &Handle{Reader: conn, Provider: "lime", Close: closeFn}, nil
}

func reservePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		return 0, err
	}
	return port, nil
}
