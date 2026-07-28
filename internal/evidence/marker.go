package evidence

import "time"

// RemoteMarker binds temporary remote files to one case. Cleanup uses the
// locally retained marker hash before it removes any remote path.
type RemoteMarker struct {
	Version      int       `json:"version"`
	CaseID       string    `json:"case_id"`
	EvidenceID   string    `json:"evidence_id"`
	AgentPath    string    `json:"agent_path"`
	RemoveAgent  bool      `json:"remove_agent"`
	ToolPaths    []string  `json:"tool_paths,omitempty"`
	PriorMarkers []string  `json:"prior_marker_paths,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}
