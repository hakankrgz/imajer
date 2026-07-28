package fsutil

type Details struct {
	Path       string `json:"path"`
	FileSystem string `json:"file_system"`
	Available  uint64 `json:"available_bytes"`
}
