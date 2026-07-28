//go:build windows

package probe

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

func IdentifyDisk(path string) (DiskIdentity, error) {
	quoted := strings.ReplaceAll(path, "'", "''")
	script := fmt.Sprintf(`$d=Get-CimInstance Win32_DiskDrive | Where-Object {$_.DeviceID -eq '%s'} | Select-Object -First 1; if($null -eq $d){exit 3}; [pscustomobject]@{Path=$d.DeviceID;IDs=@($d.DeviceID,$d.SerialNumber,$d.PNPDeviceID);Serial=$d.SerialNumber;Model=$d.Model;Size=[int64]$d.Size;SectorSize=[int64]$d.BytesPerSector}|ConvertTo-Json -Compress`, quoted)
	encoded := base64.StdEncoding.EncodeToString(utf16LE(script))
	out, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand", encoded).Output()
	if err != nil {
		return DiskIdentity{}, err
	}
	var identity DiskIdentity
	if err := json.Unmarshal(out, &identity); err != nil {
		return DiskIdentity{}, err
	}
	return identity, nil
}

func utf16LE(s string) []byte {
	out := make([]byte, 0, len(s)*2)
	for _, r := range []rune(s) {
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
