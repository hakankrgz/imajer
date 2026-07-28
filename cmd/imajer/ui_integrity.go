package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hakankrgz/imajer/internal/evidence"
)

const (
	maxUIArtifacts = 32
	maxUISessions  = 1024
	maxUIJSONSize  = 4 << 20
)

type uiIntegritySummary struct {
	Artifacts []uiArtifactIntegrity `json:"artifacts"`
}

type uiArtifactIntegrity struct {
	ArtifactID       string               `json:"artifact_id"`
	Kind             string               `json:"kind"`
	Status           string               `json:"status"`
	Verification     string               `json:"verification,omitempty"`
	SourceSize       int64                `json:"source_size"`
	ReceivedSize     int64                `json:"received_size"`
	LogicalSHA256    string               `json:"logical_sha256,omitempty"`
	RemoteFullSHA256 string               `json:"remote_full_sha256,omitempty"`
	MerkleRoot       string               `json:"merkle_root,omitempty"`
	Chunks           int                  `json:"chunks"`
	Segments         int                  `json:"segments"`
	Resumed          bool                 `json:"resumed"`
	RetryCount       int                  `json:"retry_count"`
	ContinuousMatch  bool                 `json:"continuous_match"`
	Sessions         []uiSessionIntegrity `json:"sessions,omitempty"`
}

type uiSessionIntegrity struct {
	ID           string `json:"id"`
	StartOffset  int64  `json:"start_offset"`
	EndOffset    int64  `json:"end_offset"`
	Bytes        int64  `json:"bytes"`
	RemoteSHA256 string `json:"remote_sha256,omitempty"`
	LocalSHA256  string `json:"local_sha256,omitempty"`
	Match        bool   `json:"match"`
	Provider     string `json:"provider,omitempty"`
	Error        string `json:"error,omitempty"`
}

type uiArtifactManifest struct {
	Version  int            `json:"version"`
	State    evidence.State `json:"state"`
	Segments []any          `json:"segments"`
	Chunks   int            `json:"chunks"`
}

func loadUIIntegrity(caseDir string) (*uiIntegritySummary, error) {
	if strings.TrimSpace(caseDir) == "" {
		return nil, nil
	}
	artifactsDir := filepath.Join(caseDir, "artifacts")
	entries, err := os.ReadDir(artifactsDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	summary := &uiIntegritySummary{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if len(summary.Artifacts) >= maxUIArtifacts {
			return summary, fmt.Errorf("arayüz özeti en fazla %d artifact gösterebilir", maxUIArtifacts)
		}
		dir := filepath.Join(artifactsDir, entry.Name())
		raw, err := readUIFileLimited(filepath.Join(dir, "artifact-manifest.json"), maxUIJSONSize)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return summary, err
		}
		var manifest uiArtifactManifest
		if err := json.Unmarshal(raw, &manifest); err != nil {
			return summary, fmt.Errorf("%s manifesti: %w", entry.Name(), err)
		}
		state := manifest.State
		artifact := uiArtifactIntegrity{
			ArtifactID: state.ArtifactID, Kind: state.Kind, Status: string(state.Status),
			Verification: state.Verification, SourceSize: state.SourceSize,
			ReceivedSize: state.NextOffset, LogicalSHA256: state.LogicalSHA256,
			MerkleRoot: state.MerkleRoot, Chunks: manifest.Chunks,
			Segments: len(manifest.Segments), Resumed: state.Resumed,
			RetryCount: state.RetryCount,
		}
		if state.Status == evidence.StatusVerifiedContinuous && !state.Resumed {
			artifact.RemoteFullSHA256 = state.RemoteStreamHash
			artifact.ContinuousMatch = validMatchingHashes(state.LogicalSHA256, state.RemoteStreamHash)
		}
		sessions, err := readUISessions(filepath.Join(dir, "sessions.jsonl"))
		if err != nil {
			return summary, fmt.Errorf("%s oturumları: %w", entry.Name(), err)
		}
		artifact.Sessions = sessions
		summary.Artifacts = append(summary.Artifacts, artifact)
	}
	if len(summary.Artifacts) == 0 {
		return nil, nil
	}
	return summary, nil
}

func readUIFileLimited(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("%s güvenli boyut sınırını aşıyor", filepath.Base(path))
	}
	raw := make([]byte, info.Size())
	if _, err := io.ReadFull(file, raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func readUISessions(path string) ([]uiSessionIntegrity, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var sessions []uiSessionIntegrity
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		if len(sessions) >= maxUISessions {
			return nil, fmt.Errorf("oturum sayısı %d sınırını aşıyor", maxUISessions)
		}
		var record evidence.SessionRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, err
		}
		sessions = append(sessions, uiSessionIntegrity{
			ID: record.ID, StartOffset: record.StartOffset, EndOffset: record.EndOffset,
			Bytes: record.Bytes, RemoteSHA256: record.RemoteSHA256,
			LocalSHA256: record.LocalSHA256,
			Match:       validMatchingHashes(record.RemoteSHA256, record.LocalSHA256),
			Provider:    record.Provider, Error: record.Error,
		})
	}
	return sessions, scanner.Err()
}

func validMatchingHashes(left, right string) bool {
	if len(left) != 64 || len(right) != 64 || !strings.EqualFold(left, right) {
		return false
	}
	leftRaw, leftErr := hex.DecodeString(left)
	rightRaw, rightErr := hex.DecodeString(right)
	return leftErr == nil && rightErr == nil && len(leftRaw) == 32 && len(rightRaw) == 32
}
