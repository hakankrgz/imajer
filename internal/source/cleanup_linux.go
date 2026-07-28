//go:build linux

package source

import (
	"os"
)

func CleanupFootprint() CleanupResult {
	var result CleanupResult
	if _, err := os.Stat("/sys/module/lime"); err == nil {
		result.Actions = append(result.Actions, "LiME module is loaded; automatic removal skipped because ownership cannot be proven")
	}
	return result
}
