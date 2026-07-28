package probe

type DiskIdentity struct {
	Path       string   `json:"path"`
	IDs        []string `json:"ids"`
	Serial     string   `json:"serial,omitempty"`
	Model      string   `json:"model,omitempty"`
	Size       int64    `json:"size"`
	SectorSize int64    `json:"sector_size"`
}
