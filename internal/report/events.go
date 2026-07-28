package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Event struct {
	Time       time.Time      `json:"time"`
	Level      string         `json:"level"`
	Type       string         `json:"type"`
	CaseID     string         `json:"case_id,omitempty"`
	ArtifactID string         `json:"artifact_id,omitempty"`
	Message    string         `json:"message"`
	Fields     map[string]any `json:"fields,omitempty"`
}

type EventLogger struct {
	mu sync.Mutex
	f  *os.File
}

func OpenEvents(caseDir string) (*EventLogger, error) {
	if err := os.MkdirAll(caseDir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(caseDir, "events.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &EventLogger{f: f}, nil
}

func (l *EventLogger) Log(e Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	e.Time = time.Now().UTC()
	e.Message = redact(e.Message)
	e.Fields = redactFields(e.Fields)
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if _, err := l.f.Write(append(raw, '\n')); err != nil {
		return err
	}
	return l.f.Sync()
}

func redactFields(fields map[string]any) map[string]any {
	if fields == nil {
		return nil
	}
	out := make(map[string]any, len(fields))
	for key, value := range fields {
		lower := strings.ToLower(key)
		if sensitiveKey(lower) {
			out[key] = "[REDACTED]"
			continue
		}
		out[key] = redactValue(value)
	}
	return out
}

func sensitiveKey(key string) bool {
	for _, marker := range []string{
		"password", "passphrase", "secret", "token", "private_key",
		"privatekey", "api_key", "apikey", "authorization", "credential",
	} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}

func redactValue(value any) any {
	switch v := value.(type) {
	case string:
		return redact(v)
	case map[string]any:
		return redactFields(v)
	case []any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = redactValue(v[i])
		}
		return out
	case []string:
		out := make([]string, len(v))
		for i := range v {
			out[i] = redact(v[i])
		}
		return out
	default:
		return value
	}
}

func (l *EventLogger) Close() error {
	if l == nil || l.f == nil {
		return nil
	}
	return l.f.Close()
}

func redact(s string) string {
	s = credentialAssignment.ReplaceAllString(s, "${1}=[REDACTED]")
	return bearerCredential.ReplaceAllString(s, "Bearer [REDACTED]")
}

var (
	credentialAssignment = regexp.MustCompile(`(?i)\b(password|passphrase|secret|token|api[_-]?key|authorization|credential)\s*[:=]\s*[^ \t\r\n&,;]+`)
	bearerCredential     = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]+`)
)
