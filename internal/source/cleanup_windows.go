//go:build windows

package source

func CleanupFootprint() CleanupResult {
	return CleanupResult{Actions: []string{
		"Global WinPmem residual cleanup skipped because ownership cannot be proven; the active acquisition handle removes its exact service and driver on close",
	}}
}
