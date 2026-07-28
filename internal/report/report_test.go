package report

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
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
