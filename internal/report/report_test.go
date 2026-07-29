package report

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hakankrgz/imajer/internal/config"
	"github.com/hakankrgz/imajer/internal/probe"
)

func TestIndexSignAndVerify(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "case")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "evidence.001"), []byte("forensic bytes"), 0o600)
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := x509.MarshalPKCS8PrivateKey(priv)
	keyPath := filepath.Join(root, "key.pem")
	_ = os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: raw}), 0o600)
	pubRaw, _ := x509.MarshalPKIXPublicKey(priv.Public())
	pubPath := filepath.Join(root, "trusted-public.pem")
	_ = os.WriteFile(pubPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubRaw}), 0o644)
	if _, err := FinalizeIndex(dir, "case", "evidence", keyPath); err != nil {
		t.Fatal(err)
	}
	indexRaw, err := os.ReadFile(filepath.Join(dir, "evidence-index.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(indexRaw) == 0 || indexRaw[len(indexRaw)-1] == '\n' {
		t.Fatal("evidence index is not stored as exact compact canonical bytes")
	}
	if err := VerifyIndex(dir, pubPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "evidence-index.json"), append(indexRaw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyIndex(dir, pubPath); err == nil {
		t.Fatal("non-canonical index representation was accepted")
	}
	if err := os.WriteFile(filepath.Join(dir, "evidence-index.json"), indexRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "unsigned-extra"), []byte("unexpected"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyIndex(dir, pubPath); err == nil {
		t.Fatal("unsigned extra file was not detected")
	}
}

func TestFinalizeIndexRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	caseDir := filepath.Join(root, "case")
	if err := os.Mkdir(caseDir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "outside")
	if err := os.WriteFile(target, []byte("outside evidence tree"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(caseDir, "linked-evidence")); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(root, "key.pem")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := FinalizeIndex(caseDir, "CASE", "EVID", keyPath); err == nil {
		t.Fatal("symlink in evidence tree was accepted")
	}
}

func TestReportLinesSummarizeStorageAndPreserveTurkish(t *testing.T) {
	data := CaseReport{
		Case: config.Case{Examiner: "Şule Işık"},
		Target: probe.Info{
			Storage: json.RawMessage(`{
				"blockdevices": [
					{"name":"loop0","type":"loop","size":4096,"alignment":0},
					{"name":"nvme0n1","type":"disk","size":1073741824,"model":"Örnek Disk","serial":"ABC123","alignment":0,
					 "children":[{"name":"nvme0n1p1","type":"part","size":1073741824}]}
				]
			}`),
		},
	}
	lines := reportLines(data)
	joined := strings.Join(lines, "\n")
	for _, expected := range []string{
		"İMAJER - UZAK ADLİ İMAJ ALMA RAPORU",
		"İncelemeci: Şule Işık",
		"Hedef depolama özeti:",
		"/dev/nvme0n1 | boyut=1.00 GiB (1073741824 byte) | model=Örnek Disk | seri=ABC123",
		"Ayrıntılı ham envanter: target-probe.json",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("report is missing %q:\n%s", expected, joined)
		}
	}
	for _, unwanted := range []string{`"alignment"`, "loop0", `"children"`} {
		if strings.Contains(joined, unwanted) {
			t.Fatalf("report contains raw storage detail %q:\n%s", unwanted, joined)
		}
	}
}

func TestReportTextRemovesControlCharacters(t *testing.T) {
	got := sanitizeReportText("unexpected EOF\x00wait:\nremote\tcommand")
	if got != "unexpected EOF wait: remote command" {
		t.Fatalf("unexpected sanitized text: %q", got)
	}
}

func TestPDFEscapeEncodesTurkishGlyphs(t *testing.T) {
	got := []byte(pdfEscape(`ĞğİıŞş ÇçÖöÜü (test)\path`))
	want := []byte{128, 129, 130, 131, 132, 133, ' ', 199, 231, 214, 246, 220, 252, ' ', '\\', '(', 't', 'e', 's', 't', '\\', ')', '\\', '\\', 'p', 'a', 't', 'h'}
	if !bytes.Equal(got, want) {
		t.Fatalf("unexpected PDF encoding:\n got: %v\nwant: %v", got, want)
	}
}

func TestAppendWrappedDoesNotSplitUTF8(t *testing.T) {
	lines := appendWrapped(nil, strings.Repeat("ş", 106), 105)
	if len(lines) != 2 || !utf8.ValidString(lines[0]) || !utf8.ValidString(lines[1]) {
		t.Fatalf("wrapped lines are invalid: %#v", lines)
	}
	if len([]rune(lines[0])) != 105 || lines[1] != "ş" {
		t.Fatalf("unexpected wrapping: lengths=%d/%d", len([]rune(lines[0])), len([]rune(lines[1])))
	}
}
