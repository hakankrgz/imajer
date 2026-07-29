package probe

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type Info struct {
	ProtocolVersion int             `json:"protocol_version"`
	Hostname        string          `json:"hostname"`
	OS              string          `json:"os"`
	Arch            string          `json:"arch"`
	Kernel          string          `json:"kernel,omitempty"`
	Admin           bool            `json:"admin"`
	MemoryBytes     int64           `json:"memory_bytes,omitempty"`
	UTC             time.Time       `json:"utc"`
	Tools           map[string]Tool `json:"tools"`
	Storage         json.RawMessage `json:"storage,omitempty"`
	Warnings        []string        `json:"warnings,omitempty"`
}

type Tool struct {
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
}

func Collect() Info {
	host, _ := os.Hostname()
	info := Info{
		ProtocolVersion: 1, Hostname: host, OS: runtime.GOOS, Arch: runtime.GOARCH,
		UTC: time.Now().UTC(), Tools: map[string]Tool{},
	}
	for _, name := range toolNames() {
		if path, err := exec.LookPath(name); err == nil {
			info.Tools[name] = Tool{Path: path, Version: toolVersion(path)}
		}
	}
	info.Admin = isAdmin()
	info.MemoryBytes = physicalMemory()
	info.Kernel, info.Storage, info.Warnings = platformDetails()
	if info.OS == "linux" && info.Arch != "amd64" && info.Arch != "arm64" {
		info.Warnings = append(info.Warnings, "Bundled AVML supports amd64 and arm64; this target requires another signed RAM provider")
	}
	return info
}

func toolVersion(path string) string {
	for _, args := range [][]string{{"--version"}, {"-version"}, {"/?"}} {
		cmd := exec.Command(path, args...)
		var b bytes.Buffer
		cmd.Stdout, cmd.Stderr = &b, &b
		if err := cmd.Run(); err == nil {
			line := strings.TrimSpace(b.String())
			if i := strings.IndexByte(line, '\n'); i >= 0 {
				line = line[:i]
			}
			if len(line) > 256 {
				line = line[:256]
			}
			return line
		}
	}
	return "present; version unavailable"
}
