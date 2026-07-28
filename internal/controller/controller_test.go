package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hakankrgz/imajer/internal/config"
	"github.com/hakankrgz/imajer/internal/evidence"
	"github.com/hakankrgz/imajer/internal/protocol"
	"github.com/hakankrgz/imajer/internal/report"
	"github.com/hakankrgz/imajer/internal/transport"
)

func TestDiskResumeAfterInterruptedSession(t *testing.T) {
	data := bytes.Repeat([]byte("forensic-stream-"), (3<<20)/16)
	data = data[:3<<20]
	caseDir := t.TempDir()
	events, err := report.OpenEvents(caseDir)
	if err != nil {
		t.Fatal(err)
	}
	defer events.Close()
	fake := &faultTransport{data: data, failSessions: 1}
	job := &config.Job{
		Case:        config.Case{ID: "CASE", EvidenceID: "EVID"},
		Acquisition: config.Acquisition{ChunkSize: 1 << 20, SegmentSize: 2 << 20},
		Retry:       config.Retry{MaxAttempts: 2},
	}
	c := &Controller{Job: job, Transport: fake, CaseDir: caseDir, Events: events, AgentPath: "fake-agent"}
	src := config.Source{Path: "/dev/fake", ID: "disk-serial", Size: int64(len(data)), SectorSize: 512, Provider: "native"}
	state, err := c.acquireArtifact(context.Background(), "disk", "disk", src, false)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != evidence.StatusChunkVerifiedComposite || !state.Resumed || state.SessionCount != 2 {
		t.Fatalf("unexpected state: %#v", state)
	}
	sum := sha256.Sum256(data)
	if state.LogicalSHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("logical hash mismatch: %s", state.LogicalSHA256)
	}
	if fake.starts != 2 {
		t.Fatalf("wanted two sessions, got %d", fake.starts)
	}
}

func TestRAMRestartsAtZeroAfterInterruption(t *testing.T) {
	data := bytes.Repeat([]byte("volatile-memory-"), (2<<20)/16)
	data = data[:2<<20]
	caseDir := t.TempDir()
	events, err := report.OpenEvents(caseDir)
	if err != nil {
		t.Fatal(err)
	}
	defer events.Close()
	fake := &faultTransport{data: data, failSessions: 1}
	job := &config.Job{
		Case: config.Case{ID: "CASE", EvidenceID: "EVID"},
		Acquisition: config.Acquisition{
			ChunkSize: 1 << 20, SegmentSize: 2 << 20,
			RAM: config.Source{ID: "volatile-memory", Provider: "direct"},
		},
		Retry: config.Retry{MaxAttempts: 2},
	}
	c := &Controller{Job: job, Transport: fake, CaseDir: caseDir, Events: events, AgentPath: "fake-agent"}
	state, err := c.acquireRAM(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != evidence.StatusVerifiedContinuous || state.Resumed {
		t.Fatalf("unexpected final RAM state: %#v", state)
	}
	entries, err := os.ReadDir(filepath.Join(caseDir, "artifacts"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("wanted partial and completed attempts, got %d", len(entries))
	}
	first, err := evidence.LoadState(filepath.Join(caseDir, "artifacts", "memory-attempt-001"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != evidence.StatusIncomplete || first.NextOffset == 0 {
		t.Fatalf("first RAM attempt was not preserved: %#v", first)
	}
}

func TestSessionCountIncludesEveryInterruptedStream(t *testing.T) {
	data := bytes.Repeat([]byte("session-count---"), (4<<20)/16)
	data = data[:4<<20]
	caseDir := t.TempDir()
	events, err := report.OpenEvents(caseDir)
	if err != nil {
		t.Fatal(err)
	}
	defer events.Close()
	fake := &faultTransport{data: data, failSessions: 2}
	job := &config.Job{
		Case:        config.Case{ID: "CASE", EvidenceID: "EVID"},
		Acquisition: config.Acquisition{ChunkSize: 1 << 20, SegmentSize: 2 << 20},
		Retry:       config.Retry{MaxAttempts: 3},
	}
	c := &Controller{Job: job, Transport: fake, CaseDir: caseDir, Events: events, AgentPath: "fake-agent"}
	src := config.Source{Path: "/dev/fake", ID: "disk-serial", Size: int64(len(data)), SectorSize: 512, Provider: "native"}
	state, err := c.acquireArtifact(context.Background(), "disk", "disk", src, false)
	if err != nil {
		t.Fatal(err)
	}
	if state.SessionCount != 3 || !state.Resumed || state.Status != evidence.StatusChunkVerifiedComposite {
		t.Fatalf("unexpected multi-session state: %#v", state)
	}
}

func TestRetryBudgetResetsAfterVerifiedForwardProgress(t *testing.T) {
	data := bytes.Repeat([]byte("per-chunk-retry!"), (4<<20)/16)
	data = data[:4<<20]
	caseDir := t.TempDir()
	events, err := report.OpenEvents(caseDir)
	if err != nil {
		t.Fatal(err)
	}
	defer events.Close()
	fake := &faultTransport{data: data, failSessions: 2}
	job := &config.Job{
		Case:        config.Case{ID: "CASE", EvidenceID: "EVID"},
		Acquisition: config.Acquisition{ChunkSize: 1 << 20, SegmentSize: 2 << 20},
		Retry:       config.Retry{MaxAttempts: 2},
	}
	c := &Controller{Job: job, Transport: fake, CaseDir: caseDir, Events: events, AgentPath: "fake-agent"}
	src := config.Source{Path: "/dev/fake", ID: "disk-serial", Size: int64(len(data)), SectorSize: 512, Provider: "native"}
	state, err := c.acquireArtifact(context.Background(), "disk", "disk", src, false)
	if err != nil {
		t.Fatal(err)
	}
	if state.NextOffset != int64(len(data)) || state.RetryCount != 2 ||
		state.Status != evidence.StatusChunkVerifiedComposite {
		t.Fatalf("retry budget did not reset after progress: %#v", state)
	}
}

func TestACKLossReplayIsVerifiedWithoutRewriting(t *testing.T) {
	data := bytes.Repeat([]byte("ack-loss-replay!"), (3<<20)/16)
	data = data[:3<<20]
	caseDir := t.TempDir()
	events, err := report.OpenEvents(caseDir)
	if err != nil {
		t.Fatal(err)
	}
	defer events.Close()
	fake := &faultTransport{data: data, failSessions: 1, replayVerifiedOnSecond: true}
	job := &config.Job{
		Case:        config.Case{ID: "CASE", EvidenceID: "EVID"},
		Acquisition: config.Acquisition{ChunkSize: 1 << 20, SegmentSize: 2 << 20},
		Retry:       config.Retry{MaxAttempts: 2},
	}
	c := &Controller{Job: job, Transport: fake, CaseDir: caseDir, Events: events, AgentPath: "fake-agent"}
	src := config.Source{Path: "/dev/fake", ID: "disk-serial", Size: int64(len(data)), SectorSize: 512, Provider: "native"}
	state, err := c.acquireArtifact(context.Background(), "disk", "disk", src, false)
	if err != nil {
		t.Fatal(err)
	}
	records, err := evidence.ReadChunks(filepath.Join(caseDir, "artifacts", "disk"))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 || state.NextOffset != int64(len(data)) {
		t.Fatalf("replayed chunk was appended or offset changed: records=%d state=%#v", len(records), state)
	}
}

func TestStateRecoversForwardFromVerifiedChunkJournal(t *testing.T) {
	data := bytes.Repeat([]byte("journal-recover!"), (3<<20)/16)
	data = data[:3<<20]
	caseDir := t.TempDir()
	artifactDir := filepath.Join(caseDir, "artifacts", "disk")
	if err := os.MkdirAll(artifactDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writer, err := evidence.NewSegmentedWriter(artifactDir, "disk", 2<<20)
	if err != nil {
		t.Fatal(err)
	}
	first := data[:1<<20]
	if err := writer.WriteAt(first, 0); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	chunks, err := evidence.OpenChunkLog(artifactDir)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(first)
	if err := chunks.Append(evidence.ChunkRecord{
		SessionID: "crashed-session", Offset: 0, Length: len(first),
		SHA256: hex.EncodeToString(sum[:]), ReadAt: time.Now().UTC(), WrittenAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := chunks.Close(); err != nil {
		t.Fatal(err)
	}
	state := &evidence.State{
		Version: 1, CaseID: "CASE", EvidenceID: "EVID", ArtifactID: "disk", Kind: "disk",
		SourceID: "disk-serial", SourcePath: "/dev/fake", SourceSize: int64(len(data)),
		SectorSize: 512, ChunkSize: 1 << 20, SegmentSize: 2 << 20,
		NextOffset: 0, SessionCount: 1, Status: evidence.StatusRunning, StartedAt: time.Now().UTC(),
	}
	if err := evidence.SaveState(artifactDir, state); err != nil {
		t.Fatal(err)
	}
	events, err := report.OpenEvents(caseDir)
	if err != nil {
		t.Fatal(err)
	}
	defer events.Close()
	fake := &faultTransport{data: data}
	job := &config.Job{
		Case:        config.Case{ID: "CASE", EvidenceID: "EVID"},
		Acquisition: config.Acquisition{ChunkSize: 1 << 20, SegmentSize: 2 << 20},
		Retry:       config.Retry{MaxAttempts: 1},
	}
	c := &Controller{Job: job, Transport: fake, CaseDir: caseDir, Events: events, AgentPath: "fake-agent"}
	src := config.Source{Path: "/dev/fake", ID: "disk-serial", Size: int64(len(data)), SectorSize: 512, Provider: "native"}
	got, err := c.acquireArtifact(context.Background(), "disk", "disk", src, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.NextOffset != int64(len(data)) || !got.Resumed || got.Status != evidence.StatusChunkVerifiedComposite {
		t.Fatalf("journal recovery failed: %#v", got)
	}
}

func TestLogicalChunkReassembledFromTransportSubframes(t *testing.T) {
	data := bytes.Repeat([]byte("winrm-subframe!!"), (3<<20)/16)
	data = data[:3<<20]
	caseDir := t.TempDir()
	events, err := report.OpenEvents(caseDir)
	if err != nil {
		t.Fatal(err)
	}
	defer events.Close()
	fake := &faultTransport{data: data, frameSize: 64 << 10}
	job := &config.Job{
		Case:        config.Case{ID: "CASE", EvidenceID: "EVID"},
		Acquisition: config.Acquisition{ChunkSize: 1 << 20, SegmentSize: 2 << 20},
		Retry:       config.Retry{MaxAttempts: 1},
	}
	c := &Controller{Job: job, Transport: fake, CaseDir: caseDir, Events: events, AgentPath: "fake-agent"}
	src := config.Source{Path: "/dev/fake", ID: "disk-serial", Size: int64(len(data)), SectorSize: 512, Provider: "native"}
	state, err := c.acquireArtifact(context.Background(), "disk", "disk", src, false)
	if err != nil {
		t.Fatal(err)
	}
	records, err := evidence.ReadChunks(filepath.Join(caseDir, "artifacts", "disk"))
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != evidence.StatusVerifiedContinuous || len(records) != 3 {
		t.Fatalf("subframes were not committed as three logical chunks: state=%#v records=%d", state, len(records))
	}
}

func TestFinalizeIncompleteArtifactWritesAuditableManifest(t *testing.T) {
	caseDir := t.TempDir()
	dir := filepath.Join(caseDir, "artifacts", "memory-attempt-001")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	data := bytes.Repeat([]byte{0x5a}, 1<<20)
	writer, err := evidence.NewSegmentedWriter(dir, "memory", 2<<20)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteAt(data, 0); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	chunks, err := evidence.OpenChunkLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := chunks.Append(evidence.ChunkRecord{
		SessionID: "interrupted", Offset: 0, Length: len(data),
		SHA256: hex.EncodeToString(sum[:]), ReadAt: time.Now().UTC(), WrittenAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := chunks.Close(); err != nil {
		t.Fatal(err)
	}
	state := &evidence.State{
		CaseID: "CASE", EvidenceID: "EVID", ArtifactID: "memory-attempt-001",
		Kind: "ram", SourceID: "volatile-memory", ChunkSize: 1 << 20,
		SegmentSize: 2 << 20, NextOffset: int64(len(data)),
		Status: evidence.StatusIncomplete, Verification: "incomplete",
	}
	if err := FinalizeNonCompleteArtifact(caseDir, state); err != nil {
		t.Fatal(err)
	}
	if state.LogicalSHA256 != hex.EncodeToString(sum[:]) || state.MerkleRoot == "" {
		t.Fatalf("partial integrity metadata missing: %#v", state)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "artifact-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest ArtifactManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.State.Status != evidence.StatusIncomplete || manifest.Chunks != 1 ||
		len(manifest.Segments) != 1 {
		t.Fatalf("unexpected partial manifest: %#v", manifest)
	}
}

type faultTransport struct {
	mu                     sync.Mutex
	starts                 int
	failSessions           int
	replayVerifiedOnSecond bool
	frameSize              int64
	data                   []byte
}

func (f *faultTransport) Name() string { return "fault" }
func (f *faultTransport) Close() error { return nil }
func (f *faultTransport) Upload(context.Context, string, string, uint32) error {
	return errors.New("not implemented")
}
func (f *faultTransport) HashFile(context.Context, string) (string, error) {
	return "", errors.New("not implemented")
}
func (f *faultTransport) Remove(context.Context, string) error { return nil }

func (f *faultTransport) Start(_ context.Context, argv []string) (*transport.Session, error) {
	f.mu.Lock()
	f.starts++
	sessionNo := f.starts
	f.mu.Unlock()
	offset := argInt64(argv, "--offset")
	chunkSize := argInt64(argv, "--chunk-size")
	caseID, artifactID, sourceID := argValue(argv, "--case"), argValue(argv, "--artifact"), argValue(argv, "--source-id")
	stdoutR, stdoutW := io.Pipe()
	ackR, ackW := io.Pipe()
	done := make(chan error, 1)
	go func() {
		enc := protocol.NewEncoder(stdoutW)
		h := sha256.New()
		current := offset
		var seq uint64
		if sessionNo == 2 && f.replayVerifiedOnSecond {
			chunk := f.data[:chunkSize]
			sum := sha256.Sum256(chunk)
			err := enc.Write(protocol.Header{
				Type: "data", CaseID: caseID, ArtifactID: artifactID, SourceID: sourceID,
				SessionID: "session-" + strconv.Itoa(sessionNo), Sequence: seq,
				Offset: 0, ReadAt: time.Now().UTC(), SHA256: hex.EncodeToString(sum[:]),
			}, chunk)
			if err != nil {
				done <- err
				_ = stdoutW.CloseWithError(err)
				return
			}
			if err := protocol.ReadAck(ackR); err != nil {
				done <- err
				_ = stdoutW.CloseWithError(err)
				return
			}
			seq++
		}
		for current < int64(len(f.data)) {
			end := current + chunkSize
			if end > int64(len(f.data)) {
				end = int64(len(f.data))
			}
			chunk := f.data[current:end]
			sum := sha256.Sum256(chunk)
			_, _ = h.Write(chunk)
			frameSize := int64(len(chunk))
			if f.frameSize > 0 {
				frameSize = f.frameSize
			}
			frameCount := (len(chunk) + int(frameSize) - 1) / int(frameSize)
			for frameIndex, frameStart := 0, 0; frameStart < len(chunk); frameIndex, frameStart = frameIndex+1, frameStart+int(frameSize) {
				frameEnd := frameStart + int(frameSize)
				if frameEnd > len(chunk) {
					frameEnd = len(chunk)
				}
				payload := chunk[frameStart:frameEnd]
				frameSum := sha256.Sum256(payload)
				err := enc.Write(protocol.Header{
					Type: "data", CaseID: caseID, ArtifactID: artifactID, SourceID: sourceID,
					SessionID: "session-" + strconv.Itoa(sessionNo), Sequence: seq,
					Offset: current + int64(frameStart), ReadAt: time.Now().UTC(),
					SHA256:      hex.EncodeToString(frameSum[:]),
					ChunkOffset: current, ChunkLength: len(chunk), ChunkSHA256: hex.EncodeToString(sum[:]),
					FrameIndex: frameIndex, FrameCount: frameCount,
				}, payload)
				if err != nil {
					done <- err
					_ = stdoutW.CloseWithError(err)
					return
				}
				if err := protocol.ReadAck(ackR); err != nil {
					done <- err
					_ = stdoutW.CloseWithError(err)
					return
				}
				seq++
			}
			current = end
			if sessionNo <= f.failSessions {
				err := io.ErrUnexpectedEOF
				done <- err
				_ = stdoutW.CloseWithError(err)
				return
			}
		}
		err := enc.Write(protocol.Header{
			Type: "trailer", CaseID: caseID, ArtifactID: artifactID, SourceID: sourceID,
			SessionID: "session-" + strconv.Itoa(sessionNo), Sequence: seq,
			Offset: current, Bytes: current - offset, StreamSHA256: hex.EncodeToString(h.Sum(nil)),
		}, nil)
		done <- err
		_ = stdoutW.CloseWithError(err)
	}()
	return &transport.Session{
		Stdin: ackW, Stdout: stdoutR, Stderr: strings.NewReader(""),
		Wait: func() error { return <-done },
		Close: func() error {
			_ = ackW.Close()
			_ = ackR.Close()
			_ = stdoutR.Close()
			return nil
		},
	}, nil
}

func argValue(argv []string, key string) string {
	for i := range argv {
		if argv[i] == key && i+1 < len(argv) {
			return argv[i+1]
		}
	}
	return ""
}

func argInt64(argv []string, key string) int64 {
	n, _ := strconv.ParseInt(argValue(argv, key), 10, 64)
	return n
}
