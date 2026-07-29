package transport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/hakankrgz/imajer/internal/config"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	sshagent "golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

type sshTransport struct {
	client    *ssh.Client
	sftp      *sftp.Client
	agentConn net.Conn
}

func newSSH(ctx context.Context, target config.Target, connectTimeout time.Duration) (Transport, error) {
	if connectTimeout <= 0 {
		connectTimeout = 30 * time.Second
	}
	cb, err := knownhosts.New(target.KnownHosts)
	if err != nil {
		return nil, fmt.Errorf("known_hosts: %w", err)
	}
	var auth []ssh.AuthMethod
	password := config.Password(target)
	if target.PrivateKey != "" {
		raw, err := os.ReadFile(target.PrivateKey)
		if err != nil {
			return nil, fmt.Errorf("read SSH key: %w", err)
		}
		var signer ssh.Signer
		if password != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(raw, []byte(password))
		} else {
			signer, err = ssh.ParsePrivateKey(raw)
		}
		if err != nil {
			return nil, fmt.Errorf("parse SSH key: %w", err)
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}
	var agentConn net.Conn
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if c, err := net.Dial("unix", sock); err == nil {
			agentConn = c
			auth = append(auth, ssh.PublicKeysCallback(sshagent.NewClient(c).Signers))
		}
	}
	if password != "" && target.PrivateKey == "" {
		auth = append(auth, ssh.Password(password))
	}
	if len(auth) == 0 {
		return nil, errors.New("no SSH authentication method available")
	}
	cfg := &ssh.ClientConfig{
		User: target.User, Auth: auth, HostKeyCallback: cb,
		Timeout: connectTimeout,
	}
	dialer := net.Dialer{Timeout: connectTimeout}
	rawConn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(target.Host, fmt.Sprint(target.Port)))
	if err != nil {
		if agentConn != nil {
			agentConn.Close()
		}
		return nil, err
	}
	conn, chans, reqs, err := ssh.NewClientConn(rawConn, net.JoinHostPort(target.Host, fmt.Sprint(target.Port)), cfg)
	if err != nil {
		rawConn.Close()
		if agentConn != nil {
			agentConn.Close()
		}
		return nil, err
	}
	client := ssh.NewClient(conn, chans, reqs)
	sf, err := sftp.NewClient(client)
	if err != nil {
		client.Close()
		if agentConn != nil {
			agentConn.Close()
		}
		return nil, err
	}
	return &sshTransport{client: client, sftp: sf, agentConn: agentConn}, nil
}

func (t *sshTransport) Name() string { return "ssh" }

func (t *sshTransport) Close() error {
	var agentErr error
	if t.agentConn != nil {
		agentErr = t.agentConn.Close()
		t.agentConn = nil
	}
	return errors.Join(t.sftp.Close(), t.client.Close(), agentErr)
}

func (t *sshTransport) Start(ctx context.Context, argv []string) (*Session, error) {
	if len(argv) == 0 {
		return nil, errors.New("empty command")
	}
	s, err := t.client.NewSession()
	if err != nil {
		return nil, err
	}
	stdin, err := s.StdinPipe()
	if err != nil {
		s.Close()
		return nil, err
	}
	stdout, err := s.StdoutPipe()
	if err != nil {
		s.Close()
		return nil, err
	}
	stderr, err := s.StderrPipe()
	if err != nil {
		s.Close()
		return nil, err
	}
	command, err := quotePOSIX(argv)
	if err != nil {
		s.Close()
		return nil, err
	}
	if err := s.Start(command); err != nil {
		s.Close()
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = s.Signal(ssh.SIGTERM)
			_ = s.Close()
		case <-done:
		}
	}()
	return &Session{
		Stdin: stdin, Stdout: stdout, Stderr: stderr,
		Wait: func() error {
			err := s.Wait()
			close(done)
			return err
		},
		Close: s.Close,
	}, nil
}

func (t *sshTransport) Upload(ctx context.Context, localPath, remotePath string, mode uint32) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := t.sftp.MkdirAll(remoteDir(remotePath, "/")); err != nil {
		return err
	}
	if err := t.sftp.Chmod(remoteDir(remotePath, "/"), 0o700); err != nil {
		return err
	}
	in, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := remotePath + ".upload"
	out, err := t.sftp.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		out.Close()
		if !ok {
			_ = t.sftp.Remove(tmp)
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Chmod(os.FileMode(mode)); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := t.sftp.Rename(tmp, remotePath); err != nil {
		return err
	}
	ok = true
	return nil
}

func (t *sshTransport) HashFile(ctx context.Context, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	f, err := t.sftp.Open(path)
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

func (t *sshTransport) Remove(_ context.Context, path string) error {
	err := t.sftp.Remove(path)
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "no such file") {
		return err
	}
	dir := remoteDir(path, "/")
	if dir != "/" && dir != "." {
		_ = t.sftp.RemoveDirectory(dir)
	}
	return nil
}

func quotePOSIX(argv []string) (string, error) {
	var out []string
	for _, arg := range argv {
		if strings.ContainsAny(arg, "\x00\r\n") {
			return "", errors.New("command argument contains a forbidden control character")
		}
		out = append(out, "'"+strings.ReplaceAll(arg, "'", "'\"'\"'")+"'")
	}
	return strings.Join(out, " "), nil
}

func remoteDir(path, separator string) string {
	i := strings.LastIndex(path, separator)
	if i < 0 {
		return "."
	}
	if i == 0 {
		return separator
	}
	return path[:i]
}
