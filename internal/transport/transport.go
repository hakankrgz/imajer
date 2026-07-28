package transport

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/hakankrgz/imajer/internal/config"
)

type Session struct {
	Stdin  io.WriteCloser
	Stdout io.Reader
	Stderr io.Reader
	Wait   func() error
	Close  func() error
}

type Transport interface {
	Start(ctx context.Context, argv []string) (*Session, error)
	Upload(ctx context.Context, localPath, remotePath string, mode uint32) error
	HashFile(ctx context.Context, remotePath string) (string, error)
	Remove(ctx context.Context, remotePath string) error
	Close() error
	Name() string
}

func New(ctx context.Context, target config.Target, connectTimeout time.Duration) (Transport, error) {
	switch strings.ToLower(target.Transport) {
	case "local":
		return newLocal(), nil
	case "ssh":
		return newSSH(ctx, target, connectTimeout)
	case "winrm":
		return newWinRM(ctx, target, connectTimeout)
	default:
		return nil, errors.New("unsupported transport")
	}
}
