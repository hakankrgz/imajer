//go:build windows

package probe

import (
	"encoding/json"
	"os/exec"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"
)

func toolNames() []string { return []string{"powershell.exe", "winpmem.exe", "ftkimager.exe"} }

func isAdmin() bool {
	sid, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return false
	}
	member, err := windows.Token(0).IsMember(sid)
	return err == nil && member
}

func physicalMemory() int64 {
	out, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `(Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory`).Output()
	if err != nil {
		return 0
	}
	value, _ := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	return value
}

func platformDetails() (string, json.RawMessage, []string) {
	script := `$os=Get-CimInstance Win32_OperatingSystem; $disks=Get-CimInstance Win32_DiskDrive | Select-Object DeviceID,SerialNumber,Model,Size,BytesPerSector; [pscustomobject]@{OS=$os.Caption;Version=$os.Version;Build=$os.BuildNumber;Disks=$disks;Volumes=(Get-Volume | Select-Object DriveLetter,FileSystem,Size,SizeRemaining,Path)} | ConvertTo-Json -Depth 5 -Compress`
	out, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return "", nil, []string{"PowerShell/CIM storage inventory unavailable"}
	}
	var raw json.RawMessage = out
	var header struct {
		OS, Version, Build string
	}
	_ = json.Unmarshal(out, &header)
	return strings.TrimSpace(header.OS + " " + header.Version + " build " + header.Build), raw, nil
}
