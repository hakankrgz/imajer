package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEventRedaction(t *testing.T) {
	dir := t.TempDir()
	log, err := OpenEvents(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := log.Log(Event{
		Level: "error", Type: "test",
		Message: "password=hunter2 passphrase:open api_key=xyz Authorization=BasicValue Bearer ey.token.value",
		Fields: map[string]any{
			"private_key": "raw-key",
			"detail":      "secret=value",
			"nested": []any{
				map[string]any{"credential": "nested-secret"},
				"token=inside-list",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "hunter2") || strings.Contains(text, "raw-key") ||
		strings.Contains(text, "secret=value") || strings.Contains(text, "open") ||
		strings.Contains(text, "api_key=xyz") || strings.Contains(text, "BasicValue") ||
		strings.Contains(text, "ey.token.value") || strings.Contains(text, "nested-secret") ||
		strings.Contains(text, "inside-list") {
		t.Fatalf("secret leaked: %s", text)
	}
}
