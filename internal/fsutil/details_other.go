//go:build !darwin && !windows

package fsutil

func Inspect(path string) (Details, error) {
	available, err := Available(path)
	if err != nil {
		return Details{}, err
	}
	return Details{Path: path, FileSystem: "UNKNOWN", Available: available}, nil
}
