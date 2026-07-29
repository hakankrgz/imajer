//go:build windows

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func browseNativePath(ctx context.Context, kind, currentPath string) (string, bool, error) {
	script := `[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false); ` +
		`Add-Type -AssemblyName System.Windows.Forms; ` +
		`[System.Windows.Forms.Application]::EnableVisualStyles(); ` +
		`$initial=$env:IMAJER_BROWSE_INITIAL; ` +
		`if($initial -and [IO.File]::Exists($initial)){$initial=[IO.Path]::GetDirectoryName($initial)}; ` +
		`if(-not ($initial -and [IO.Directory]::Exists($initial))){$initial=[Environment]::GetFolderPath('MyDocuments')}; ` +
		`$owner=New-Object System.Windows.Forms.Form; ` +
		`$owner.StartPosition='CenterScreen'; $owner.Size=New-Object System.Drawing.Size(1,1); ` +
		`$owner.ShowInTaskbar=$false; $owner.TopMost=$true; $owner.Opacity=0; ` +
		`$owner.Show(); $owner.Activate(); `
	if kind == "directory" {
		script += `$d=New-Object System.Windows.Forms.FolderBrowserDialog; ` +
			`$d.Description='Bir klasör seçin'; ` +
			`if($initial){$d.SelectedPath=$initial}; ` +
			`$result=$d.ShowDialog($owner); $owner.Close(); ` +
			`if($result -eq 'OK'){[Console]::Write($d.SelectedPath); exit 0}; exit 2`
	} else {
		script += `$d=New-Object System.Windows.Forms.OpenFileDialog; ` +
			`$d.Title='Bir dosya seçin'; $d.CheckFileExists=$true; $d.RestoreDirectory=$true; ` +
			`if($initial){$d.InitialDirectory=$initial}; ` +
			`$result=$d.ShowDialog($owner); $owner.Close(); ` +
			`if($result -eq 'OK'){[Console]::Write($d.FileName); exit 0}; exit 2`
	}
	cmd := exec.CommandContext(
		ctx, "powershell.exe",
		"-NoLogo", "-NoProfile", "-STA", "-NonInteractive", "-WindowStyle", "Hidden",
		"-Command", script,
	)
	cmd.Env = append(os.Environ(), "IMAJER_BROWSE_INITIAL="+strings.TrimSpace(currentPath))
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
