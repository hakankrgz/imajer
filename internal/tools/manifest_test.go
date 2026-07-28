package tools

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestManifestSignVerify(t *testing.T) {
	dir := t.TempDir()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	privRaw, _ := x509.MarshalPKCS8PrivateKey(priv)
	pubRaw, _ := x509.MarshalPKIXPublicKey(pub)
	privPath, pubPath := filepath.Join(dir, "priv.pem"), filepath.Join(dir, "pub.pem")
	_ = os.WriteFile(privPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privRaw}), 0o600)
	_ = os.WriteFile(pubPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubRaw}), 0o600)
	artifact := filepath.Join(dir, "agent")
	_ = os.WriteFile(artifact, []byte("signed artifact"), 0o700)
	manifest := filepath.Join(dir, "manifest.json")
	a := Artifact{Name: "imajer-agent", Version: "test", OS: "linux", Arch: "amd64", Path: artifact, License: "Apache-2.0"}
	if err := Create([]Artifact{a}, privPath, manifest); err != nil {
		t.Fatal(err)
	}
	m, err := LoadAndVerify(manifest, pubPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyFile(artifact, m.Artifacts[0].SHA256); err != nil {
		t.Fatal(err)
	}
}
