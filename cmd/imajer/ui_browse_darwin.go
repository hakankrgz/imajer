//go:build darwin

package main

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

func browseNativePath(ctx context.Context, kind, _ string) (string, bool, error) {
	script := `POSIX path of (choose file with prompt "Bir dosya seçin")`
	if kind == "directory" {
		script = `POSIX path of (choose folder with prompt "Bir klasör seçin")`
	}
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.ToLower(string(output))
		if strings.Contains(message, "user canceled") || strings.Contains(message, "(-128)") {
			return "", true, nil
		}
		if ctx.Err() != nil {
			return "", false, ctx.Err()
		}
		return "", false, errors.New(strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), false, nil
}
