package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDefaultsAndDurations(t *testing.T) {
	dir := t.TempDir()
	jobPath := filepath.Join(dir, "job.yaml")
	raw := []byte(`
case:
  id: CASE
  evidence_id: EVID
  examiner: Examiner
  authority_ref: AUTH
  authorized: true
target:
  transport: local
acquisition:
  profile: disk
  disk:
    path: /dev/fake
    id: serial
    size: 1048576
output:
  directory: ` + filepath.Join(dir, "out") + `
retry:
  connect_timeout: 12s
  chunk_timeout: 2m
`)
	if err := os.WriteFile(jobPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	job, err := Load(jobPath)
	if err != nil {
		t.Fatal(err)
	}
	if job.Retry.Connect != 12*time.Second || job.Retry.Chunk != 2*time.Minute ||
		job.Retry.Cleanup != 2*time.Minute {
		t.Fatalf("durations not parsed: %#v", job.Retry)
	}
	if job.Acquisition.ChunkSize != DefaultChunkSize || job.Acquisition.SegmentSize != DefaultSegmentSize {
		t.Fatal("defaults not applied")
	}
}

func TestRejectSigningKeyInsideEvidenceTree(t *testing.T) {
	dir := t.TempDir()
	job := Job{
		Case:   Case{ID: "CASE", EvidenceID: "EVID", Examiner: "x", AuthorityRef: "a", Authorized: true},
		Target: Target{Transport: "local"},
		Acquisition: Acquisition{
			Profile: "disk", ChunkSize: DefaultChunkSize, SegmentSize: DefaultSegmentSize,
			Disk: Source{Path: "/dev/fake", ID: "serial", Size: 1 << 20},
		},
		Output: Output{Directory: dir, SigningKey: filepath.Join(dir, "private.pem")},
	}
	if err := job.Validate(); err == nil {
		t.Fatal("expected signing key location rejection")
	}
}

func TestLoadForOverridesDefersRequiredFieldValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "partial.yaml")
	raw := []byte(`
case:
  id: CASE
  evidence_id: EVID
  examiner: Examiner
  authority_ref: AUTH
  authorized: true
target:
  transport: local
acquisition:
  profile: disk
output:
  directory: ` + filepath.Join(dir, "out") + `
`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	job, err := LoadForOverrides(path)
	if err != nil {
		t.Fatal(err)
	}
	job.Acquisition.Disk = Source{Path: "/dev/fake", ID: "serial", Size: 1 << 20}
	if err := job.Validate(); err != nil {
		t.Fatalf("flag-equivalent overrides should complete the job: %v", err)
	}
}
