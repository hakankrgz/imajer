//go:build !windows

package probe

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

func toolNames() []string { return []string{"avml", "dc3dd", "dd", "insmod", "rmmod", "sha256sum"} }

func isAdmin() bool { return os.Geteuid() == 0 }

func physicalMemory() int64 {
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err == nil {
			value, _ := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
			return value
		}
	}
	raw, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kib, _ := strconv.ParseInt(fields[1], 10, 64)
				return kib * 1024
			}
		}
	}
	return 0
}

func platformDetails() (string, json.RawMessage, []string) {
	kernelRaw, _ := exec.Command("uname", "-sr").Output()
	var storage json.RawMessage
	var warnings []string
	if out, err := exec.Command("lsblk", "-b", "-J", "-O").Output(); err == nil {
		storage = bytes.TrimSpace(out)
	} else {
		warnings = append(warnings, "lsblk JSON inventory unavailable")
	}
	if raw, err := os.ReadFile("/sys/kernel/security/lockdown"); err == nil &&
		!strings.Contains(string(raw), "[none]") {
		warnings = append(warnings, "kernel lockdown may prevent AVML or LiME acquisition")
	}
	return strings.TrimSpace(string(kernelRaw)), storage, warnings
}
