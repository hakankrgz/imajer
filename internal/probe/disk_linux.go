//go:build linux

package probe

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func IdentifyDisk(path string) (DiskIdentity, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return DiskIdentity{}, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return DiskIdentity{}, err
	}
	if info.Mode().IsRegular() {
		return DiskIdentity{Path: resolved, IDs: []string{path, resolved, filepath.Base(path)}, Size: info.Size(), SectorSize: 512}, nil
	}
	name := filepath.Base(resolved)
	sys := filepath.Join("/sys/class/block", name)
	if _, err := os.Stat(sys); err != nil {
		return DiskIdentity{}, fmt.Errorf("not a Linux block device: %s", path)
	}
	sizeSectors, err := readInt(filepath.Join(sys, "size"))
	if err != nil {
		return DiskIdentity{}, err
	}
	sector, err := readInt(filepath.Join(sys, "queue", "logical_block_size"))
	if err != nil {
		sector = 512
	}
	sysSerial := readText(filepath.Join(sys, "device", "serial"))
	cid := readText(filepath.Join(sys, "device", "cid"))
	sysModel := readText(filepath.Join(sys, "device", "model"))
	deviceName := readText(filepath.Join(sys, "device", "name"))
	sysWWID := readText(filepath.Join(sys, "device", "wwid"))
	lsblkModel, lsblkSerial, lsblkWWN := lsblkIdentity(resolved)
	model, serial, _, alternateIDs := mergeLinuxDeviceMetadata(
		sysSerial, cid, sysModel, deviceName, sysWWID,
		lsblkModel, lsblkSerial, lsblkWWN,
	)
	ids := []string{path, resolved}
	for _, value := range alternateIDs {
		if value != "" {
			ids = append(ids, value)
		}
	}
	for _, link := range glob("/dev/disk/by-id/*") {
		target, err := filepath.EvalSymlinks(link)
		if err == nil && target == resolved {
			ids = append(ids, link, filepath.Base(link))
		}
	}
	return DiskIdentity{
		Path: resolved, IDs: uniqueStrings(ids), Serial: serial, Model: model,
		Size: sizeSectors * 512, SectorSize: sector,
	}, nil
}

func lsblkIdentity(path string) (model, serial, wwn string) {
	output, err := exec.Command(
		"lsblk", "-b", "-J", "-d", "-o", "MODEL,SERIAL,WWN", path,
	).Output()
	if err != nil {
		return "", "", ""
	}
	var envelope struct {
		BlockDevices []struct {
			Model  json.RawMessage `json:"model"`
			Serial json.RawMessage `json:"serial"`
			WWN    json.RawMessage `json:"wwn"`
		} `json:"blockdevices"`
	}
	if json.Unmarshal(output, &envelope) != nil || len(envelope.BlockDevices) != 1 {
		return "", "", ""
	}
	text := func(raw json.RawMessage) string {
		if len(raw) == 0 || string(raw) == "null" {
			return ""
		}
		var value string
		if json.Unmarshal(raw, &value) == nil {
			return strings.TrimSpace(value)
		}
		return strings.Trim(strings.TrimSpace(string(raw)), `"`)
	}
	row := envelope.BlockDevices[0]
	return text(row.Model), text(row.Serial), text(row.WWN)
}

func readInt(path string) (int64, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
}

func readText(path string) string {
	raw, _ := os.ReadFile(path)
	return strings.TrimSpace(string(raw))
}

func glob(pattern string) []string {
	values, _ := filepath.Glob(pattern)
	return values
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
