//go:build linux

package probe

import (
	"fmt"
	"os"
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
	serial := readText(filepath.Join(sys, "device", "serial"))
	model := readText(filepath.Join(sys, "device", "model"))
	wwid := readText(filepath.Join(sys, "device", "wwid"))
	ids := []string{path, resolved}
	for _, value := range []string{serial, wwid} {
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
