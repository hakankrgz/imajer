package source

type CleanupResult struct {
	Actions []string `json:"actions"`
	Errors  []string `json:"errors,omitempty"`
}
