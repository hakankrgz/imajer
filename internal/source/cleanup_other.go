//go:build !linux && !windows

package source

func CleanupFootprint() CleanupResult { return CleanupResult{} }
