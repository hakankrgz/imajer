package transport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
)

type localTransport struct{}

func newLocal() Transport            { return &localTransport{} }
func (*localTransport) Name() string { return "local" }
func (*localTransport) Close() error { return nil }

func (*localTransport) Start(ctx context.Context, argv []string) (*Session, error) {
	if len(argv) == 0 {
		return nil, errors.New("empty command")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &Session{
		Stdin: stdin, Stdout: stdout, Stderr: stderr,
		Wait: cmd.Wait,
		Close: func() error {
			_ = stdin.Close()
			if cmd.Process != nil {
				return cmd.Process.Kill()
			}
			return nil
		},
	}, nil
}

func (*localTransport) Upload(_ context.Context, localPath, remotePath string, mode uint32) error {
	if localPath == remotePath {
		return nil
	}
	in, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(remotePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(mode))
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		out.Close()
		if !ok {
			_ = os.Remove(remotePath)
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}

func (*localTransport) HashFile(_ context.Context, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (*localTransport) Remove(_ context.Context, path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
