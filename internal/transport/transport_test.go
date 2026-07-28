package transport

import (
	"strings"
	"testing"
)

func TestPOSIXQuoting(t *testing.T) {
	got, err := quotePOSIX([]string{"/tmp/agent", "--source", "/dev/disk by 'id'"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `'"'"'`) {
		t.Fatalf("single quote was not safely escaped: %s", got)
	}
	if _, err := quotePOSIX([]string{"agent", "bad\nargument"}); err == nil {
		t.Fatal("expected control-character rejection")
	}
}

func TestWindowsQuoting(t *testing.T) {
	got, err := quoteWindows([]string{`C:\Temp\agent.exe`, `a b`, `quote"value`, `tail\`})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `"a b"`) || !strings.Contains(got, `quote\"value`) {
		t.Fatalf("arguments were not quoted: %s", got)
	}
	if _, err := quoteWindows([]string{"agent", "bad\rargument"}); err == nil {
		t.Fatal("expected control-character rejection")
	}
}
