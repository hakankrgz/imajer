//go:build windows

package evidence

// replaceFile uses MOVEFILE_WRITE_THROUGH on Windows. Opening or flushing a
// directory handle is not consistently supported and may return access denied
// even after the file was successfully replaced.
func syncDir(string) error {
	return nil
}
