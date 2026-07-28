package tools

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type Manifest struct {
	Version   int        `json:"version"`
	CreatedAt time.Time  `json:"created_at"`
	Artifacts []Artifact `json:"artifacts"`
	Signature string     `json:"signature"`
}

type Artifact struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Kernel  string `json:"kernel,omitempty"`
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	License string `json:"license"`
}

type signedPayload struct {
	Version   int        `json:"version"`
	CreatedAt time.Time  `json:"created_at"`
	Artifacts []Artifact `json:"artifacts"`
}

func LoadAndVerify(path, publicKeyPath string) (*Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("decode tool manifest: %w", err)
	}
	if m.Version != 1 || len(m.Artifacts) == 0 {
		return nil, errors.New("invalid or empty tool manifest")
	}
	if m.CreatedAt.IsZero() {
		return nil, errors.New("tool manifest has no creation timestamp")
	}
	seen := make(map[string]struct{}, len(m.Artifacts))
	for _, a := range m.Artifacts {
		if a.Name == "" || a.Version == "" || a.OS == "" || a.Arch == "" || a.License == "" {
			return nil, errors.New("tool manifest artifact metadata is incomplete")
		}
		if a.Path == "" || filepath.Base(a.Path) != a.Path || a.Path == "." {
			return nil, fmt.Errorf("unsafe artifact path %q", a.Path)
		}
		digest, err := hex.DecodeString(a.SHA256)
		if err != nil || len(digest) != sha256.Size {
			return nil, fmt.Errorf("invalid SHA-256 for artifact %s", a.Name)
		}
		key := a.Name + "\x00" + a.OS + "\x00" + a.Arch + "\x00" + a.Kernel
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("duplicate artifact selector for %s", a.Name)
		}
		seen[key] = struct{}{}
	}
	pub, err := loadPublicKey(publicKeyPath)
	if err != nil {
		return nil, err
	}
	sig, err := base64.StdEncoding.DecodeString(m.Signature)
	if err != nil {
		return nil, errors.New("invalid manifest signature encoding")
	}
	if len(sig) != ed25519.SignatureSize {
		return nil, errors.New("invalid manifest signature size")
	}
	payload, err := canonicalPayload(m)
	if err != nil {
		return nil, err
	}
	if !ed25519.Verify(pub, payload, sig) {
		return nil, errors.New("tool manifest signature verification failed")
	}
	return &m, nil
}

func (m *Manifest) Find(name, goos, goarch, kernel string) (*Artifact, error) {
	for i := range m.Artifacts {
		a := &m.Artifacts[i]
		if a.Name == name && a.OS == goos && a.Arch == goarch &&
			(a.Kernel == "" || a.Kernel == kernel) {
			return a, nil
		}
	}
	return nil, fmt.Errorf("no signed %s artifact for %s/%s kernel %q", name, goos, goarch, kernel)
}

func VerifyFile(path, expected string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("artifact is not a regular file: %s", path)
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("SHA-256 mismatch for %s: got %s", path, actual)
	}
	return nil
}

func Create(paths []Artifact, privateKeyPath, output string) error {
	if len(paths) == 0 {
		return errors.New("at least one artifact is required")
	}
	for i := range paths {
		if paths[i].Path == "" {
			return errors.New("artifact path is required")
		}
		h, err := hashFile(paths[i].Path)
		if err != nil {
			return err
		}
		paths[i].SHA256 = h
		paths[i].Path = filepath.Base(paths[i].Path)
	}
	sort.Slice(paths, func(i, j int) bool {
		return paths[i].Name+paths[i].OS+paths[i].Arch < paths[j].Name+paths[j].OS+paths[j].Arch
	})
	m := Manifest{Version: 1, CreatedAt: time.Now().UTC(), Artifacts: paths}
	key, err := loadPrivateKey(privateKeyPath)
	if err != nil {
		return err
	}
	payload, err := canonicalPayload(m)
	if err != nil {
		return err
	}
	m.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(key, payload))
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(output, raw, 0o600)
}

func canonicalPayload(m Manifest) ([]byte, error) {
	artifacts := append([]Artifact(nil), m.Artifacts...)
	sort.Slice(artifacts, func(i, j int) bool {
		a, b := artifacts[i], artifacts[j]
		return a.Name+a.OS+a.Arch+a.Kernel+a.Path < b.Name+b.OS+b.Arch+b.Kernel+b.Path
	})
	return json.Marshal(signedPayload{Version: m.Version, CreatedAt: m.CreatedAt.UTC(), Artifacts: artifacts})
}

func loadPublicKey(path string) (ed25519.PublicKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("public key is not PEM")
	}
	v, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	pub, ok := v.(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("public key is not Ed25519")
	}
	return pub, nil
}

func loadPrivateKey(path string) (ed25519.PrivateKey, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("private key permissions are too broad; require 0600 or stricter")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("private key is not PEM")
	}
	v, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := v.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not Ed25519")
	}
	return key, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func CurrentAgentArtifact(path string) Artifact {
	return Artifact{
		Name: "imajer-agent", Version: "dev", OS: runtime.GOOS, Arch: runtime.GOARCH,
		Path: path, License: "Apache-2.0",
	}
}
