package main

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hakankrgz/imajer/internal/config"
)

func TestDerivedRemoteMarkerPath(t *testing.T) {
	hash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	tests := []struct {
		name, agent, transport, want string
	}{
		{"ssh", "/dev/shm/imajer-abcd/imajer-agent", "ssh", "/dev/shm/imajer-abcd/.imajer-case-marker-0123456789abcdef.json"},
		{"winrm", `C:\Windows\Temp\imajer-abcd\imajer-agent.exe`, "winrm", `C:\Windows\Temp\imajer-abcd\.imajer-case-marker-0123456789abcdef.json`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := derivedRemoteMarkerPath(tt.agent, tt.transport, hash); got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestUISplitProgressOnCarriageReturn(t *testing.T) {
	scanner := bufioNewScanner(strings.NewReader("first\rsecond\nthird"))
	var got []string
	for scanner.Scan() {
		got = append(got, scanner.Text())
	}
	if strings.Join(got, ",") != "first,second,third" {
		t.Fatalf("unexpected tokens: %#v", got)
	}
}

func TestUIRedaction(t *testing.T) {
	input := "password=hunter2 Authorization=BasicSecret Bearer ey.secret.token"
	got := redactUI(input)
	for _, secret := range []string{"hunter2", "BasicSecret", "ey.secret.token"} {
		if strings.Contains(got, secret) {
			t.Fatalf("secret leaked: %q", got)
		}
	}
}

func TestUISavesValidJobWithoutCredential(t *testing.T) {
	root := t.TempDir()
	server := &uiServer{workingDir: root}
	jobPath, caseDir, err := server.saveJob(uiJobForm{
		CaseID: "CASE-UI", EvidenceID: "EVID-UI", Examiner: "Examiner",
		AuthorityRef: "AUTH", Authorized: true, Transport: "local", Profile: "disk",
		DiskPath: filepath.Join(root, "source.raw"), DiskID: "source",
		DiskSize: "2097152", SectorSize: "512", DiskProvider: "native",
		OutputDirectory: filepath.Join(root, "evidence"),
		SigningKey:      filepath.Join(root, "keys", "examiner.pem"),
		AgentLocal:      filepath.Join(root, "imajer-agent"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if caseDir != filepath.Join(root, "evidence", "CASE-UI", "EVID-UI") {
		t.Fatalf("unexpected case directory: %s", caseDir)
	}
	raw, err := os.ReadFile(jobPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(bytes.ToLower(raw), []byte("password")) {
		t.Fatalf("credential field unexpectedly persisted: %s", raw)
	}
	job, err := config.Load(jobPath)
	if err != nil {
		t.Fatal(err)
	}
	if job.Retry.Cleanup.String() != "2m0s" || job.Acquisition.ChunkSize != config.DefaultChunkSize {
		t.Fatalf("defaults were not preserved: %#v", job)
	}
}

func TestUIRequiresTokenForMutation(t *testing.T) {
	server := &uiServer{token: "expected"}
	called := false
	handler := server.requireToken(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodPost, "/api/run", strings.NewReader("{}"))
	response := httptest.NewRecorder()
	handler(response, request)
	if response.Code != http.StatusForbidden || called {
		t.Fatalf("missing token was accepted: code=%d called=%v", response.Code, called)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/run", strings.NewReader("{}"))
	request.Header.Set("X-Imajer-Token", "expected")
	response = httptest.NewRecorder()
	handler(response, request)
	if response.Code != http.StatusNoContent || !called {
		t.Fatalf("valid token was rejected: code=%d called=%v", response.Code, called)
	}
}

func TestUIRejectsNonLocalHostHeader(t *testing.T) {
	called := false
	handler := localhostOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "http://attacker.example/api/config", nil)
	request.Host = "attacker.example"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || called {
		t.Fatalf("non-local Host was accepted: code=%d called=%v", response.Code, called)
	}
	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8765/api/config", nil)
	request.Host = "127.0.0.1:8765"
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || !called {
		t.Fatalf("localhost Host was rejected: code=%d called=%v", response.Code, called)
	}
}

func TestEnsureExaminerKeyCreatesAndRecoversPublicKey(t *testing.T) {
	root := t.TempDir()
	privatePath, publicPath, err := ensureExaminerKey(root)
	if err != nil {
		t.Fatal(err)
	}
	privateInfo, err := os.Stat(privatePath)
	if err != nil {
		t.Fatal(err)
	}
	if privateInfo.Mode().Perm()&0o077 != 0 {
		t.Fatalf("private key permissions too broad: %o", privateInfo.Mode().Perm())
	}
	if err := os.Remove(publicPath); err != nil {
		t.Fatal(err)
	}
	recoveredPrivate, recoveredPublic, err := ensureExaminerKey(root)
	if err != nil {
		t.Fatal(err)
	}
	if recoveredPrivate != privatePath || recoveredPublic != publicPath {
		t.Fatalf("unexpected recovered paths: %s %s", recoveredPrivate, recoveredPublic)
	}
	privateKey := readTestPrivateKey(t, privatePath)
	publicKey := readTestPublicKey(t, publicPath)
	if !privateKey.Public().(ed25519.PublicKey).Equal(publicKey) {
		t.Fatal("recovered public key does not match private key")
	}
}

func TestFindEdgeExecutableFromPath(t *testing.T) {
	binDir := t.TempDir()
	edgePath := filepath.Join(binDir, "msedge.exe")
	if err := os.WriteFile(edgePath, []byte("test"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	got, err := findEdgeExecutable()
	if err != nil {
		t.Fatal(err)
	}
	if got != edgePath {
		t.Fatalf("got %q want %q", got, edgePath)
	}
}

func readTestPrivateKey(t *testing.T, path string) ed25519.PrivateKey {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		t.Fatal("private key is not PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		t.Fatal("private key is not Ed25519")
	}
	return key
}

func readTestPublicKey(t *testing.T, path string) ed25519.PublicKey {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		t.Fatal("public key is not PEM")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	key, ok := parsed.(ed25519.PublicKey)
	if !ok {
		t.Fatal("public key is not Ed25519")
	}
	return key
}

func bufioNewScanner(reader *strings.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(reader)
	scanner.Split(splitLinesAndCarriageReturns)
	return scanner
}
