//go:build windows

package main

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"syscall"
)

func browseNativePath(ctx context.Context, kind string) (string, bool, error) {
	script := `[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false); Add-Type -AssemblyName System.Windows.Forms; `
	if kind == "directory" {
		script += `$d=New-Object System.Windows.Forms.FolderBrowserDialog; $d.Description='Bir klasör seçin'; if($d.ShowDialog() -eq 'OK'){[Console]::Write($d.SelectedPath); exit 0}; exit 2`
	} else {
		script += `$d=New-Object System.Windows.Forms.OpenFileDialog; $d.Title='Bir dosya seçin'; $d.CheckFileExists=$true; if($d.ShowDialog() -eq 'OK'){[Console]::Write($d.FileName); exit 0}; exit 2`
	}
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-STA", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 2 {
			return "", true, nil
		}
		if ctx.Err() != nil {
			return "", false, ctx.Err()
		}
		return "", false, errors.New(strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), false, nil
}
