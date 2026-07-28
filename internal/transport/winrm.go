package transport

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/hakankrgz/imajer/internal/config"
	"github.com/masterzen/winrm"
)

type winRMTransport struct {
	client *winrm.Client
}

func newWinRM(_ context.Context, target config.Target, connectTimeout time.Duration) (Transport, error) {
	var ca []byte
	var err error
	if target.CAFile != "" {
		ca, err = os.ReadFile(target.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read WinRM CA: %w", err)
		}
	}
	if target.CertFingerprint != "" && target.CAFile == "" {
		return nil, errors.New("WinRM certificate fingerprint-only mode is not available; provide ca_file")
	}
	endpoint := winrm.NewEndpoint(target.Host, target.Port, true, false, ca, nil, nil, connectTimeout)
	params := winrm.DefaultParameters
	switch strings.ToLower(target.Auth) {
	case "", "basic":
	case "ntlm":
		params.TransportDecorator = func() winrm.Transporter { return &winrm.ClientNTLM{} }
	case "kerberos":
		settings := &winrm.Settings{
			WinRMUsername: target.User, WinRMPassword: config.Password(target),
			WinRMHost: target.Host, WinRMPort: target.Port, WinRMProto: "https",
			KrbRealm: target.KerberosRealm, KrbConfig: target.KerberosConfig,
			KrbSpn: target.KerberosSPN, KrbCCache: target.KerberosCCache,
		}
		params.TransportDecorator = func() winrm.Transporter { return winrm.NewClientKerberos(settings) }
	default:
		return nil, fmt.Errorf("unsupported WinRM auth %q", target.Auth)
	}
	client, err := winrm.NewClientWithParameters(endpoint, target.User, config.Password(target), params)
	if err != nil {
		return nil, err
	}
	return &winRMTransport{client: client}, nil
}

func (*winRMTransport) Name() string { return "winrm" }
func (*winRMTransport) Close() error { return nil }

func (t *winRMTransport) Start(ctx context.Context, argv []string) (*Session, error) {
	command, err := quoteWindows(argv)
	if err != nil {
		return nil, err
	}
	shell, err := t.client.CreateShell()
	if err != nil {
		return nil, err
	}
	cmd, err := shell.ExecuteWithContext(ctx, command)
	if err != nil {
		shell.Close()
		return nil, err
	}
	var once sync.Once
	closeFn := func() error {
		var err error
		once.Do(func() {
			_ = cmd.Close()
			err = shell.Close()
		})
		return err
	}
	return &Session{
		Stdin: cmd.Stdin, Stdout: cmd.Stdout, Stderr: cmd.Stderr,
		Wait: func() error {
			cmd.Wait()
			code := cmd.ExitCode()
			_ = closeFn()
			if code != 0 {
				return fmt.Errorf("WinRM command exited with code %d", code)
			}
			return nil
		},
		Close: closeFn,
	}, nil
}

func (t *winRMTransport) Upload(ctx context.Context, localPath, remotePath string, _ uint32) error {
	raw, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return errors.New("refusing to upload an empty file")
	}
	dir := remoteDir(remotePath, `\`)
	create := fmt.Sprintf(`$ErrorActionPreference='Stop'; $d=New-Item -ItemType Directory -Force -Path '%s'; $acl=New-Object System.Security.AccessControl.DirectorySecurity; $acl.SetAccessRuleProtection($true,$false); @([System.Security.Principal.WindowsIdentity]::GetCurrent().Name,'SYSTEM','BUILTIN\Administrators') | ForEach-Object {$r=New-Object System.Security.AccessControl.FileSystemAccessRule($_,'FullControl','ContainerInherit,ObjectInherit','None','Allow');$acl.AddAccessRule($r)}; Set-Acl -LiteralPath $d.FullName -AclObject $acl; [IO.File]::WriteAllBytes('%s',[byte[]]@())`, psQuote(dir), psQuote(remotePath))
	if err := t.runPowerShell(ctx, create); err != nil {
		return err
	}
	for start := 0; start < len(raw); start += 48 << 10 {
		end := start + (48 << 10)
		if end > len(raw) {
			end = len(raw)
		}
		encoded := base64.StdEncoding.EncodeToString(raw[start:end])
		script := fmt.Sprintf(`$ErrorActionPreference='Stop'; $b=[Convert]::FromBase64String('%s'); $f=[IO.File]::Open('%s',[IO.FileMode]::Append,[IO.FileAccess]::Write,[IO.FileShare]::None); try {$f.Write($b,0,$b.Length);$f.Flush($true)} finally {$f.Dispose()}`, encoded, psQuote(remotePath))
		if err := t.runPowerShell(ctx, script); err != nil {
			_ = t.Remove(ctx, remotePath)
			return err
		}
	}
	return nil
}

func (t *winRMTransport) HashFile(ctx context.Context, path string) (string, error) {
	script := fmt.Sprintf(`$ErrorActionPreference='Stop'; (Get-FileHash -LiteralPath '%s' -Algorithm SHA256).Hash.ToLowerInvariant()`, psQuote(path))
	stdout, err := t.runPowerShellOutput(ctx, script)
	if err != nil {
		return "", err
	}
	sum := strings.TrimSpace(stdout)
	if len(sum) != 64 {
		return "", errors.New("invalid remote SHA-256 output")
	}
	return sum, nil
}

func (t *winRMTransport) Remove(ctx context.Context, path string) error {
	script := fmt.Sprintf(`$ErrorActionPreference='Stop'; if(Test-Path -LiteralPath '%s'){Remove-Item -LiteralPath '%s' -Force}; $d='%s'; if(Test-Path -LiteralPath $d){Remove-Item -LiteralPath $d -Force -ErrorAction SilentlyContinue}`, psQuote(path), psQuote(path), psQuote(remoteDir(path, `\`)))
	return t.runPowerShell(ctx, script)
}

func (t *winRMTransport) runPowerShell(ctx context.Context, script string) error {
	_, err := t.runPowerShellOutput(ctx, script)
	return err
}

func (t *winRMTransport) runPowerShellOutput(ctx context.Context, script string) (string, error) {
	encoded := base64.StdEncoding.EncodeToString(utf16LE(script))
	var stdout, stderr bytes.Buffer
	code, err := t.client.RunWithContext(ctx, "powershell.exe -NoProfile -NonInteractive -EncodedCommand "+encoded, &stdout, &stderr)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("PowerShell exited %d: %s", code, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func quoteWindows(argv []string) (string, error) {
	var b strings.Builder
	for i, arg := range argv {
		if strings.ContainsAny(arg, "\x00\r\n") {
			return "", errors.New("command argument contains a forbidden control character")
		}
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteByte('"')
		backslashes := 0
		for _, r := range arg {
			if r == '\\' {
				backslashes++
				continue
			}
			if r == '"' {
				b.WriteString(strings.Repeat(`\`, backslashes*2+1))
				b.WriteRune('"')
				backslashes = 0
				continue
			}
			b.WriteString(strings.Repeat(`\`, backslashes))
			backslashes = 0
			b.WriteRune(r)
		}
		b.WriteString(strings.Repeat(`\`, backslashes*2))
		b.WriteByte('"')
	}
	return b.String(), nil
}

func psQuote(s string) string { return strings.ReplaceAll(s, "'", "''") }

func utf16LE(s string) []byte {
	runes := []rune(s)
	out := make([]byte, 0, len(runes)*2)
	for _, r := range runes {
		if r <= 0xffff {
			out = append(out, byte(r), byte(r>>8))
			continue
		}
		r -= 0x10000
		hi := uint16(0xd800 + (r >> 10))
		lo := uint16(0xdc00 + (r & 0x3ff))
		out = append(out, byte(hi), byte(hi>>8), byte(lo), byte(lo>>8))
	}
	return out
}
