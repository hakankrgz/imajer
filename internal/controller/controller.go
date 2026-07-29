package controller

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hakankrgz/imajer/internal/config"
	"github.com/hakankrgz/imajer/internal/evidence"
	"github.com/hakankrgz/imajer/internal/fsutil"
	"github.com/hakankrgz/imajer/internal/probe"
	"github.com/hakankrgz/imajer/internal/protocol"
	"github.com/hakankrgz/imajer/internal/report"
	"github.com/hakankrgz/imajer/internal/tools"
	"github.com/hakankrgz/imajer/internal/transport"
)

type Controller struct {
	Job           *config.Job
	Transport     transport.Transport
	CaseDir       string
	Events        *report.EventLogger
	Probe         probe.Info
	LocalStorage  fsutil.Details
	Footprint     []string
	Tools         []report.ToolEvidence
	AgentPath     string
	agentUploaded bool
	uploadedTools []string
	markerPath    string
	markerHash    string
	markerPaths   []string
	useSudo       bool
}

type ArtifactManifest struct {
	Version  int                 `json:"version"`
	State    evidence.State      `json:"state"`
	Segments []evidence.FileHash `json:"segments"`
	Chunks   int                 `json:"chunks"`
}

// FinalizeNonCompleteArtifact records the hashes of bytes already committed
// for an incomplete or failed attempt. It never upgrades verification status.
func FinalizeNonCompleteArtifact(caseDir string, state *evidence.State) error {
	if state.Status == evidence.StatusVerifiedContinuous || state.Status == evidence.StatusChunkVerifiedComposite {
		return nil
	}
	dir := filepath.Join(caseDir, "artifacts", state.ArtifactID)
	base := "disk"
	if state.Kind == "ram" {
		base = "memory"
	}
	summary, err := evidence.AuditChunks(dir, base, state.SegmentSize)
	if err != nil {
		return err
	}
	if summary.EndOffset != state.NextOffset {
		return fmt.Errorf("partial artifact journal ends at %d but state says %d", summary.EndOffset, state.NextOffset)
	}
	logical, segments, err := evidence.HashLogical(dir, base, state.SegmentSize, state.NextOffset)
	if err != nil {
		return err
	}
	state.LogicalSHA256 = logical
	state.MerkleRoot = summary.MerkleRoot
	if err := evidence.SaveState(dir, state); err != nil {
		return err
	}
	manifest := ArtifactManifest{Version: 1, State: *state, Segments: segments, Chunks: summary.Count}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return evidence.AtomicWrite(filepath.Join(dir, "artifact-manifest.json"), append(raw, '\n'), 0o600)
}

const maxRAMAttempts = 3

func New(ctx context.Context, job *config.Job) (*Controller, error) {
	tr, err := connectWithRetry(ctx, job)
	if err != nil {
		return nil, err
	}
	caseDir := filepath.Join(job.Output.Directory, job.Case.ID, job.Case.EvidenceID)
	events, err := report.OpenEvents(caseDir)
	if err != nil {
		tr.Close()
		return nil, err
	}
	return &Controller{Job: job, Transport: tr, CaseDir: caseDir, Events: events}, nil
}

func connectWithRetry(ctx context.Context, job *config.Job) (transport.Transport, error) {
	attempts := max(job.Retry.MaxAttempts, 1)
	var result error
	for attempt := 1; attempt <= attempts; attempt++ {
		tr, err := transport.New(ctx, job.Target, job.Retry.Connect)
		if err == nil {
			return tr, nil
		}
		result = errors.Join(result, err)
		if strings.EqualFold(job.Target.Transport, "local") || attempt == attempts {
			break
		}
		select {
		case <-ctx.Done():
			return nil, errors.Join(result, ctx.Err())
		case <-time.After(backoff(attempt)):
		}
	}
	return nil, fmt.Errorf("connect after %d attempt(s): %w", attempts, result)
}

func (c *Controller) Close() error {
	return errors.Join(c.Events.Close(), c.Transport.Close())
}

func (c *Controller) Prepare(ctx context.Context) error {
	if c.AgentPath != "" {
		return nil
	}
	if c.Job.Target.Transport == "local" {
		if c.Job.Agent.LocalPath == "" {
			return errors.New("agent.local_path is required for local transport")
		}
		c.AgentPath = c.Job.Agent.LocalPath
	} else if c.Job.Agent.LocalPath == "" {
		if c.Job.Agent.RemotePath == "" {
			return errors.New("agent.local_path or a preinstalled agent.remote_path is required")
		}
		if c.Job.Agent.ToolManifest == "" || c.Job.Agent.TrustPublicKey == "" {
			return errors.New("preinstalled remote agent requires signed tool_manifest and trust_public_key")
		}
		c.AgentPath = c.Job.Agent.RemotePath
		manifest, err := tools.LoadAndVerify(c.Job.Agent.ToolManifest, c.Job.Agent.TrustPublicKey)
		if err != nil {
			return err
		}
		actual, err := c.remoteHash(ctx, c.AgentPath)
		if err != nil {
			return fmt.Errorf("hash preinstalled agent over trusted transport: %w", err)
		}
		artifact, err := matchingRemoteAgentArtifact(manifest, actual, c.Transport.Name())
		if err != nil {
			return err
		}
		c.Tools = append(c.Tools, report.ToolEvidence{
			Name: artifact.Name, Version: artifact.Version, OS: artifact.OS, Arch: artifact.Arch,
			Kernel: artifact.Kernel, SHA256: artifact.SHA256, License: artifact.License,
			RemotePath: c.AgentPath, Trust: "signed-manifest-and-transport-sha256",
		})
		if err := c.installRemoteMarker(ctx); err != nil {
			return fmt.Errorf("install case marker beside preinstalled agent: %w", err)
		}
	} else {
		if c.Job.Agent.ToolManifest == "" || c.Job.Agent.TrustPublicKey == "" {
			return errors.New("remote agent upload requires signed tool_manifest and trust_public_key")
		}
		manifest, err := tools.LoadAndVerify(c.Job.Agent.ToolManifest, c.Job.Agent.TrustPublicKey)
		if err != nil {
			return err
		}
		agentArtifact, err := matchingAgentArtifact(manifest, c.Job.Agent.LocalPath)
		if err != nil {
			return err
		}
		if err := tools.VerifyFile(c.Job.Agent.LocalPath, agentArtifact.SHA256); err != nil {
			return err
		}
		var candidates []string
		if c.Job.Agent.RemotePath != "" {
			candidates = []string{c.Job.Agent.RemotePath}
		} else {
			nonce, err := randomID(8)
			if err != nil {
				return err
			}
			if c.Job.Target.Transport == "winrm" {
				candidates = []string{`C:\Windows\Temp\imajer-` + nonce + `\imajer-agent.exe`}
			} else {
				candidates = []string{
					"/dev/shm/imajer-" + nonce + "/imajer-agent",
					"/tmp/imajer-" + nonce + "/imajer-agent",
				}
			}
		}
		var uploadErr error
		for _, remote := range candidates {
			if err := c.Transport.Upload(ctx, c.Job.Agent.LocalPath, remote, 0o700); err != nil {
				uploadErr = errors.Join(uploadErr, err)
				continue
			}
			c.AgentPath, c.agentUploaded = remote, true
			actual, err := c.remoteHash(ctx, remote)
			if err == nil && strings.EqualFold(actual, agentArtifact.SHA256) {
				// A successful upload does not prove that the mount permits
				// execution. Hardened Ubuntu systems may mount /dev/shm with
				// noexec, so test the candidate before committing to it and
				// fall back to /tmp when necessary.
				stdout, stderr, execErr := c.run(ctx, []string{remote, "version"})
				if execErr == nil && strings.TrimSpace(stdout) != "" {
					uploadErr = nil
					break
				}
				if execErr == nil {
					execErr = errors.New("agent execution returned an empty version")
				}
				uploadErr = errors.Join(uploadErr, fmt.Errorf(
					"candidate %s is not executable: %w: %s",
					remote, execErr, strings.TrimSpace(stderr),
				))
				_ = c.Transport.Remove(ctx, remote)
				c.AgentPath, c.agentUploaded = "", false
				continue
			}
			if err == nil {
				err = errors.New("uploaded agent hash mismatch")
			}
			uploadErr = errors.Join(uploadErr, err)
			_ = c.Transport.Remove(ctx, remote)
			c.AgentPath, c.agentUploaded = "", false
		}
		if c.AgentPath == "" {
			return fmt.Errorf("upload/execute signed agent: %w", uploadErr)
		}
		if err := c.installRemoteMarker(ctx); err != nil {
			// The exact file was created by this process and marker creation did
			// not complete, so this is a rollback rather than a general cleanup.
			_ = c.Transport.Remove(ctx, c.AgentPath)
			c.AgentPath, c.agentUploaded = "", false
			return fmt.Errorf("install remote case marker: %w", err)
		}
		c.Tools = append(c.Tools, report.ToolEvidence{
			Name: agentArtifact.Name, Version: agentArtifact.Version, OS: agentArtifact.OS,
			Arch: agentArtifact.Arch, Kernel: agentArtifact.Kernel, SHA256: agentArtifact.SHA256,
			License: agentArtifact.License, RemotePath: c.AgentPath, Trust: "signed-manifest-and-remote-sha256",
		})
		c.Footprint = append(c.Footprint, "Temporary signed agent uploaded: "+c.AgentPath)
		if err := c.Events.Log(report.Event{Level: "info", Type: "footprint", CaseID: c.Job.Case.ID, Message: "Signed agent uploaded to " + c.AgentPath}); err != nil {
			return err
		}
	}
	info, err := c.Discover(ctx)
	if err != nil {
		return err
	}
	if !info.Admin {
		if c.Job.Target.Transport != "local" || !localRegularSource(c.Job) {
			return errors.New("target session does not have required administrator/root privileges")
		}
	}
	if c.Job.Target.Transport != "local" {
		if info.OS != "linux" && info.OS != "windows" {
			return fmt.Errorf("unsupported remote target operating system %q", info.OS)
		}
		if info.OS == "windows" && info.Arch != "amd64" {
			return fmt.Errorf("Windows remote agent supports amd64 targets only, found %s", info.Arch)
		}
	}
	c.Probe = info
	if strings.EqualFold(c.Job.Acquisition.Profile, "disk") || strings.EqualFold(c.Job.Acquisition.Profile, "both") {
		if err := c.verifyDiskIdentity(ctx, c.Job.Acquisition.Disk); err != nil {
			return err
		}
	}
	if err := c.prepareSourceTool(ctx, &c.Job.Acquisition.RAM); err != nil {
		return err
	}
	if err := c.prepareSourceTool(ctx, &c.Job.Acquisition.Disk); err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(info, "", "  ")
	if err := evidence.AtomicWrite(filepath.Join(c.CaseDir, "target-probe.json"), append(raw, '\n'), 0o600); err != nil {
		return err
	}
	return nil
}

func (c *Controller) verifyDiskIdentity(ctx context.Context, src config.Source) error {
	if c.Job.Target.Transport == "local" && localRegularSource(c.Job) {
		return nil
	}
	stdout, stderr, err := c.run(ctx, c.agentCommand("identify", "--path", src.Path))
	if err != nil {
		return fmt.Errorf("disk identity probe: %w: %s", err, strings.TrimSpace(stderr))
	}
	var identity probe.DiskIdentity
	if err := json.Unmarshal([]byte(stdout), &identity); err != nil {
		return err
	}
	matched := false
	for _, id := range identity.IDs {
		if src.ID == id || (c.Probe.OS == "windows" && strings.EqualFold(src.ID, id)) {
			matched = true
			break
		}
	}
	if !matched {
		return fmt.Errorf("selected disk stable ID %q does not match target identifiers %v", src.ID, identity.IDs)
	}
	if identity.Size != src.Size {
		return fmt.Errorf("selected disk size changed: job=%d target=%d", src.Size, identity.Size)
	}
	if src.SectorSize > 0 && identity.SectorSize != src.SectorSize {
		return fmt.Errorf("selected disk sector size changed: job=%d target=%d", src.SectorSize, identity.SectorSize)
	}
	if src.Model != "" && !strings.EqualFold(strings.TrimSpace(identity.Model), strings.TrimSpace(src.Model)) {
		return fmt.Errorf("selected disk model changed: job=%q target=%q", src.Model, identity.Model)
	}
	return nil
}

func (c *Controller) prepareSourceTool(ctx context.Context, src *config.Source) error {
	if src.ToolLocalPath == "" {
		return nil
	}
	if src.ToolName == "" {
		return errors.New("tool_name is required with tool_local_path")
	}
	if c.Job.Agent.ToolManifest == "" || c.Job.Agent.TrustPublicKey == "" {
		return errors.New("tool upload requires signed tool_manifest and trust_public_key")
	}
	manifest, err := tools.LoadAndVerify(c.Job.Agent.ToolManifest, c.Job.Agent.TrustPublicKey)
	if err != nil {
		return err
	}
	var artifact *tools.Artifact
	for i := range manifest.Artifacts {
		a := &manifest.Artifacts[i]
		if a.Name != src.ToolName || a.OS != c.Probe.OS || a.Arch != c.Probe.Arch {
			continue
		}
		if a.Kernel != "" && a.Kernel != c.Probe.Kernel {
			continue
		}
		if err := tools.VerifyFile(src.ToolLocalPath, a.SHA256); err == nil {
			artifact = a
			break
		}
	}
	if artifact == nil {
		return fmt.Errorf("signed tool %s does not match target %s/%s kernel %q", src.ToolName, c.Probe.OS, c.Probe.Arch, c.Probe.Kernel)
	}
	remote := src.ToolRemotePath
	if remote == "" {
		remote = remoteJoin(remoteParent(c.AgentPath, c.Transport.Name()), filepath.Base(src.ToolLocalPath), c.Transport.Name())
	}
	c.uploadedTools = append(c.uploadedTools, remote)
	if !strings.EqualFold(c.Job.Target.Transport, "local") {
		if err := c.installRemoteMarker(ctx); err != nil {
			c.uploadedTools = c.uploadedTools[:len(c.uploadedTools)-1]
			return fmt.Errorf("update remote case marker for tool %s: %w", src.ToolName, err)
		}
	}
	if err := c.Transport.Upload(ctx, src.ToolLocalPath, remote, 0o700); err != nil {
		return fmt.Errorf("upload signed tool %s: %w", src.ToolName, err)
	}
	actual, err := c.remoteHash(ctx, remote)
	if err != nil || !strings.EqualFold(actual, artifact.SHA256) {
		_ = c.Transport.Remove(ctx, remote)
		if err != nil {
			return err
		}
		return fmt.Errorf("uploaded tool %s hash mismatch", src.ToolName)
	}
	src.ToolPath = remote
	c.Tools = append(c.Tools, report.ToolEvidence{
		Name: artifact.Name, Version: artifact.Version, OS: artifact.OS, Arch: artifact.Arch,
		Kernel: artifact.Kernel, SHA256: artifact.SHA256, License: artifact.License,
		RemotePath: remote, Trust: "signed-manifest-and-remote-sha256",
	})
	c.Footprint = append(c.Footprint, "Temporary signed tool uploaded: "+remote)
	return c.Events.Log(report.Event{
		Level: "info", Type: "footprint", CaseID: c.Job.Case.ID,
		Message: "Signed tool uploaded: " + remote,
		Fields:  map[string]any{"tool": src.ToolName, "version": artifact.Version, "sha256": artifact.SHA256, "license": artifact.License},
	})
}

func localRegularSource(job *config.Job) bool {
	if strings.ToLower(job.Acquisition.Profile) != "disk" {
		return false
	}
	info, err := os.Stat(job.Acquisition.Disk.Path)
	return err == nil && info.Mode().IsRegular()
}

func (c *Controller) Discover(ctx context.Context) (probe.Info, error) {
	if c.AgentPath == "" {
		if c.Job.Target.Transport == "local" {
			c.AgentPath = c.Job.Agent.LocalPath
		} else {
			c.AgentPath = c.Job.Agent.RemotePath
		}
	}
	if c.AgentPath == "" {
		return probe.Info{}, errors.New("agent path is not prepared")
	}
	stdout, stderr, err := c.run(ctx, []string{c.AgentPath, "probe"})
	if err != nil {
		return probe.Info{}, fmt.Errorf("agent probe: %w: %s", err, strings.TrimSpace(stderr))
	}
	var info probe.Info
	if err := json.Unmarshal([]byte(stdout), &info); err != nil {
		return probe.Info{}, fmt.Errorf("decode probe: %w", err)
	}
	if !info.Admin && info.OS == "linux" && c.Transport.Name() == "ssh" {
		stdout, stderr, err = c.run(ctx, []string{"sudo", "-n", c.AgentPath, "probe"})
		if err == nil {
			var elevated probe.Info
			if decodeErr := json.Unmarshal([]byte(stdout), &elevated); decodeErr != nil {
				return probe.Info{}, fmt.Errorf("decode elevated probe: %w", decodeErr)
			}
			if elevated.Admin {
				c.useSudo = true
				info = elevated
			}
		}
	}
	return info, nil
}

func (c *Controller) Acquire(ctx context.Context) ([]evidence.State, error) {
	if err := c.Prepare(ctx); err != nil {
		return nil, err
	}
	if err := c.checkLocalSpace(); err != nil {
		return nil, err
	}
	if err := c.Events.Log(report.Event{Level: "info", Type: "case_started", CaseID: c.Job.Case.ID, Message: "Authorized acquisition started"}); err != nil {
		return nil, err
	}
	var states []evidence.State
	profile := strings.ToLower(c.Job.Acquisition.Profile)
	if profile == "ram" || profile == "both" {
		state, err := c.acquireRAM(ctx)
		if state != nil {
			states = append(states, *state)
		}
		if err != nil {
			return states, err
		}
	}
	if profile == "disk" || profile == "both" {
		state, err := c.acquireArtifact(ctx, "disk", "disk", c.Job.Acquisition.Disk, false)
		if state != nil {
			states = append(states, *state)
		}
		if err != nil {
			return states, err
		}
	}
	return states, nil
}

func (c *Controller) checkLocalSpace() error {
	details, err := fsutil.Inspect(c.CaseDir)
	if err != nil {
		return fmt.Errorf("local free-space check: %w", err)
	}
	c.LocalStorage = details
	available := details.Available
	var required int64
	profile := strings.ToLower(c.Job.Acquisition.Profile)
	if profile == "disk" || profile == "both" {
		required += c.Job.Acquisition.Disk.Size
	}
	if profile == "ram" || profile == "both" {
		ramSize := c.Job.Acquisition.RAM.Size
		if ramSize == 0 {
			ramSize = c.Probe.MemoryBytes
		}
		required += ramSize * maxRAMAttempts
	}
	if required == 0 {
		return nil
	}
	// Five percent covers manifests, partial boundary segments and reports.
	required += required / 20
	if uint64(required) > available {
		return fmt.Errorf("insufficient local evidence space: require %s including reserve, have %s",
			humanBytes(required), humanBytes(int64(available)))
	}
	return nil
}

func (c *Controller) acquireRAM(ctx context.Context) (*evidence.State, error) {
	var last *evidence.State
	src := c.Job.Acquisition.RAM
	for attempt := 1; attempt <= maxRAMAttempts; attempt++ {
		id := fmt.Sprintf("memory-attempt-%03d", nextRAMAttempt(c.CaseDir))
		state, err := c.acquireArtifact(ctx, id, "ram", src, true)
		last = state
		if err == nil {
			return state, nil
		}
		canFallbackToLiME := (src.Provider == "" || strings.EqualFold(src.Provider, "auto")) &&
			strings.EqualFold(c.Probe.OS, "linux") &&
			strings.EqualFold(filepath.Ext(src.ToolPath), ".ko")
		if state == nil {
			return state, err
		}
		state.Status = evidence.StatusIncomplete
		state.Verification = "incomplete"
		state.LastError = err.Error()
		state.CompletedAt = time.Now().UTC()
		if saveErr := evidence.SaveState(filepath.Join(c.CaseDir, "artifacts", id), state); saveErr != nil {
			return state, errors.Join(err, saveErr)
		}
		if logErr := c.Events.Log(report.Event{
			Level: "warning", Type: "ram_attempt_incomplete", CaseID: c.Job.Case.ID,
			ArtifactID: id, Message: err.Error(), Fields: map[string]any{"bytes": state.NextOffset},
		}); logErr != nil {
			return state, errors.Join(err, logErr)
		}
		if canFallbackToLiME {
			src.Provider = "lime"
			if logErr := c.Events.Log(report.Event{
				Level: "warning", Type: "provider_fallback", CaseID: c.Job.Case.ID,
				ArtifactID: id, Message: "AVML attempt failed; next zero-offset RAM attempt will use the signed LiME module",
			}); logErr != nil {
				return state, errors.Join(err, logErr)
			}
		}
		if attempt < maxRAMAttempts {
			delay := backoff(attempt)
			if err := c.waitAndReconnect(ctx, delay); err != nil {
				if logErr := c.Events.Log(report.Event{
					Level: "warning", Type: "reconnect_failed", CaseID: c.Job.Case.ID,
					ArtifactID: id, Message: err.Error(), Fields: map[string]any{"delay": delay.String()},
				}); logErr != nil {
					return state, errors.Join(err, logErr)
				}
			}
		}
	}
	return last, errors.New("RAM acquisition failed after three zero-offset attempts")
}

func (c *Controller) acquireArtifact(ctx context.Context, artifactID, kind string, src config.Source, ram bool) (*evidence.State, error) {
	dir := filepath.Join(c.CaseDir, "artifacts", artifactID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	base := "disk"
	if ram {
		base = "memory"
	}
	state, err := c.loadOrCreateState(dir, artifactID, kind, src)
	if err != nil {
		return nil, err
	}
	if state.Status == evidence.StatusVerifiedContinuous || state.Status == evidence.StatusChunkVerifiedComposite {
		return state, nil
	}
	summary, err := evidence.AuditChunks(dir, base, state.SegmentSize)
	if err != nil {
		return state, err
	}
	if summary.EndOffset < state.NextOffset {
		return state, fmt.Errorf("chunk log ends at %d but state says %d", summary.EndOffset, state.NextOffset)
	}
	if src.Size > 0 && summary.EndOffset > src.Size {
		return state, fmt.Errorf("verified chunk log exceeds source size: %d > %d", summary.EndOffset, src.Size)
	}
	if summary.EndOffset > state.NextOffset {
		old := state.NextOffset
		state.NextOffset = summary.EndOffset
		state.Resumed = true
		if err := evidence.SaveState(dir, state); err != nil {
			return state, err
		}
		if err := c.Events.Log(report.Event{
			Level: "warning", Type: "state_recovered", CaseID: c.Job.Case.ID, ArtifactID: artifactID,
			Message: "Atomic state recovered from the verified chunk journal",
			Fields:  map[string]any{"old_offset": old, "recovered_offset": summary.EndOffset},
		}); err != nil {
			return state, err
		}
	}
	writer, err := evidence.NewSegmentedWriter(dir, base, state.SegmentSize)
	if err != nil {
		return state, err
	}
	defer writer.Close()
	chunkLog, err := evidence.OpenChunkLog(dir)
	if err != nil {
		return state, err
	}
	defer chunkLog.Close()

	maxAttempts := max(c.Job.Retry.MaxAttempts, 1)
	failuresAtOffset := 0
	retryOffset := state.NextOffset
	for {
		if kind == "disk" && state.NextOffset > 0 &&
			(c.Transport.Name() == "ssh" || c.Transport.Name() == "winrm") {
			if err := c.verifyDiskIdentity(ctx, src); err != nil {
				return state, err
			}
		}
		if state.NextOffset > 0 && state.SessionCount > 0 {
			state.Resumed = true
		}
		session, sessionErr := c.transferSession(ctx, state, src, writer, chunkLog, dir)
		if session != nil {
			if appendErr := evidence.AppendSession(dir, *session); appendErr != nil {
				return state, errors.Join(sessionErr, fmt.Errorf("append mandatory session record: %w", appendErr))
			}
		}
		if sessionErr == nil {
			if kind == "ram" && state.SourceSize == 0 {
				state.SourceSize = state.NextOffset
			}
			total := state.NextOffset
			logical, segments, err := evidence.HashLogical(dir, base, state.SegmentSize, total)
			if err != nil {
				return state, err
			}
			summary, err = evidence.AuditChunks(dir, base, state.SegmentSize)
			if err != nil {
				return state, err
			}
			if summary.EndOffset != total {
				return state, fmt.Errorf("verified chunk journal ends at %d, expected %d", summary.EndOffset, total)
			}
			state.LogicalSHA256, state.MerkleRoot = logical, summary.MerkleRoot
			if !state.Resumed && state.SessionCount == 1 {
				if !strings.EqualFold(logical, state.RemoteStreamHash) {
					return state, errors.New("continuous remote stream and independent local SHA-256 differ")
				}
				state.Status = evidence.StatusVerifiedContinuous
				state.Verification = "verified_continuous"
			} else {
				state.Status = evidence.StatusChunkVerifiedComposite
				state.Verification = "chunk_verified_composite"
			}
			state.CompletedAt = time.Now().UTC()
			state.LastError = ""
			if err := evidence.SaveState(dir, state); err != nil {
				return state, err
			}
			manifest := ArtifactManifest{Version: 1, State: *state, Segments: segments, Chunks: summary.Count}
			raw, _ := json.MarshalIndent(manifest, "", "  ")
			if err := evidence.AtomicWrite(filepath.Join(dir, "artifact-manifest.json"), append(raw, '\n'), 0o600); err != nil {
				return state, err
			}
			if err := c.Events.Log(report.Event{Level: "info", Type: "artifact_verified", CaseID: c.Job.Case.ID, ArtifactID: artifactID, Message: string(state.Status)}); err != nil {
				return state, err
			}
			fmt.Fprintln(os.Stderr)
			return state, nil
		}
		state.LastError = sessionErr.Error()
		if err := evidence.SaveState(dir, state); err != nil {
			return state, errors.Join(sessionErr, err)
		}
		if ram {
			return state, sessionErr
		}
		if state.NextOffset != retryOffset {
			// Forward progress starts a fresh retry budget for the next
			// uncommitted logical chunk.
			retryOffset = state.NextOffset
			failuresAtOffset = 0
		}
		failuresAtOffset++
		state.RetryCount++
		if strings.EqualFold(src.Provider, "auto") || src.Provider == "" {
			msg := strings.ToLower(sessionErr.Error())
			switch {
			case strings.Contains(msg, "dc3dd failed"):
				src.Provider = "dd"
			case strings.Contains(msg, "dd failed"):
				src.Provider = "native"
			}
		} else if strings.EqualFold(src.Provider, "dd") &&
			strings.Contains(strings.ToLower(sessionErr.Error()), "dd failed") {
			src.Provider = "native"
		}
		if failuresAtOffset >= maxAttempts {
			state.Status = evidence.StatusFailed
			if err := evidence.SaveState(dir, state); err != nil {
				return state, errors.Join(sessionErr, err)
			}
			return state, fmt.Errorf("disk remains resumable after %d failures at offset %d: %w",
				failuresAtOffset, state.NextOffset, sessionErr)
		}
		delay := backoff(failuresAtOffset)
		if err := c.Events.Log(report.Event{
			Level: "warning", Type: "retry", CaseID: c.Job.Case.ID, ArtifactID: artifactID,
			Message: sessionErr.Error(),
			Fields: map[string]any{
				"failure_at_offset": failuresAtOffset, "total_retries": state.RetryCount,
				"offset": state.NextOffset, "delay": delay.String(),
			},
		}); err != nil {
			return state, err
		}
		select {
		case <-ctx.Done():
			return state, ctx.Err()
		case <-time.After(delay):
		}
		if err := c.reconnectTransport(ctx); err != nil {
			state.LastError = "transport reconnect failed: " + err.Error()
			if saveErr := evidence.SaveState(dir, state); saveErr != nil {
				return state, errors.Join(err, saveErr)
			}
			if logErr := c.Events.Log(report.Event{
				Level: "warning", Type: "reconnect_failed", CaseID: c.Job.Case.ID, ArtifactID: artifactID,
				Message: err.Error(),
				Fields: map[string]any{
					"failure_at_offset": failuresAtOffset, "total_retries": state.RetryCount,
					"offset": state.NextOffset,
				},
			}); logErr != nil {
				return state, errors.Join(err, logErr)
			}
		}
	}
}

func (c *Controller) waitAndReconnect(ctx context.Context, delay time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
	}
	return c.reconnectTransport(ctx)
}

func (c *Controller) reconnectTransport(ctx context.Context) error {
	if c.Transport == nil || c.Transport.Name() == "local" {
		return nil
	}
	_ = c.Transport.Close()
	tr, err := transport.New(ctx, c.Job.Target, c.Job.Retry.Connect)
	if err != nil {
		return err
	}
	c.Transport = tr
	return nil
}

func (c *Controller) transferSession(ctx context.Context, state *evidence.State, src config.Source, writer *evidence.SegmentedWriter, chunks *evidence.ChunkLog, dir string) (*evidence.SessionRecord, error) {
	start := state.NextOffset
	args := c.agentCommand(
		"stream",
		"--case", c.Job.Case.ID, "--artifact", state.ArtifactID,
		"--kind", state.Kind, "--source-id", state.SourceID,
		"--source", src.Path, "--provider", defaultProvider(src.Provider),
		"--offset", strconv.FormatInt(start, 10),
		"--size", strconv.FormatInt(src.Size, 10),
		"--sector-size", strconv.FormatInt(max(src.SectorSize, 512), 10),
		"--chunk-size", strconv.FormatInt(state.ChunkSize, 10),
	)
	if src.ToolPath != "" {
		args = append(args, "--tool-path", src.ToolPath)
	}
	if c.Transport.Name() == "winrm" {
		args = append(args, "--frame-size", strconv.FormatInt(64<<10, 10))
	}
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	proc, err := c.Transport.Start(sessionCtx, args)
	if err != nil {
		return nil, err
	}
	defer proc.Close()
	var stderr strings.Builder
	stderrDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&limitedWriter{W: &stderr, N: 1 << 20}, proc.Stderr)
		close(stderrDone)
	}()
	decoder := protocol.NewDecoder(proc.Stdout)
	localSessionHash := sha256.New()
	session := &evidence.SessionRecord{StartedAt: time.Now().UTC(), StartOffset: start}
	sessionStart := time.Now()
	var expectedSequence uint64
	var assembled []byte
	var assembledOffset int64
	var assembledDigest string
	var assembledReadAt time.Time
	var assembledSequence uint64
	var expectedFrameIndex int
	var expectedFrameCount int
	var replay bool
	for {
		frame, err := readFrameWithTimeout(decoder, c.Job.Retry.Chunk)
		if err != nil {
			_ = proc.Stdin.Close()
			_ = proc.Close()
			waitErr := proc.Wait()
			<-stderrDone
			session.EndedAt, session.EndOffset = time.Now().UTC(), state.NextOffset
			session.LocalSHA256 = hex.EncodeToString(localSessionHash.Sum(nil))
			session.Bytes = state.NextOffset - start
			session.Error = errors.Join(err, waitErr).Error()
			return session, fmt.Errorf("stream interrupted: %w: %s", errors.Join(err, waitErr), strings.TrimSpace(stderr.String()))
		}
		h := frame.Header
		if h.CaseID != c.Job.Case.ID || h.ArtifactID != state.ArtifactID || h.SourceID != state.SourceID {
			_ = protocol.WriteAck(proc.Stdin, false)
			return session, errors.New("frame identity mismatch")
		}
		if session.ID == "" {
			session.ID = h.SessionID
			state.SessionCount++
			if err := evidence.SaveState(dir, state); err != nil {
				return session, err
			}
		} else if session.ID != h.SessionID {
			return session, errors.New("session ID changed within stream")
		}
		switch h.Type {
		case "data":
			session.Provider = h.Provider
			if h.Sequence != expectedSequence {
				_ = protocol.WriteAck(proc.Stdin, false)
				return session, fmt.Errorf("unexpected frame sequence: got %d want %d", h.Sequence, expectedSequence)
			}
			sum := sha256.Sum256(frame.Payload)
			localHash := hex.EncodeToString(sum[:])
			if !strings.EqualFold(localHash, h.SHA256) {
				_ = protocol.WriteAck(proc.Stdin, false)
				return session, fmt.Errorf("chunk SHA-256 mismatch at %d", h.Offset)
			}
			logicalOffset, logicalLength, logicalDigest := h.ChunkOffset, h.ChunkLength, h.ChunkSHA256
			frameIndex, frameCount := h.FrameIndex, h.FrameCount
			if logicalLength == 0 {
				logicalOffset, logicalLength, logicalDigest = h.Offset, len(frame.Payload), h.SHA256
				frameIndex, frameCount = 0, 1
			}
			if logicalLength <= 0 || logicalLength > int(state.ChunkSize) || frameCount <= 0 {
				_ = protocol.WriteAck(proc.Stdin, false)
				return session, errors.New("invalid logical chunk metadata")
			}
			if assembled == nil {
				if logicalOffset > state.NextOffset {
					_ = protocol.WriteAck(proc.Stdin, false)
					return session, fmt.Errorf("non-contiguous logical chunk: got %d want %d", logicalOffset, state.NextOffset)
				}
				replay = logicalOffset < state.NextOffset
				assembled = make([]byte, 0, logicalLength)
				assembledOffset, assembledDigest, assembledReadAt = logicalOffset, logicalDigest, h.ReadAt
				assembledSequence = h.Sequence
				expectedFrameIndex, expectedFrameCount = 0, frameCount
			}
			if logicalOffset != assembledOffset || logicalLength != cap(assembled) ||
				!strings.EqualFold(logicalDigest, assembledDigest) ||
				frameIndex != expectedFrameIndex || frameCount != expectedFrameCount ||
				h.Offset != assembledOffset+int64(len(assembled)) {
				_ = protocol.WriteAck(proc.Stdin, false)
				return session, errors.New("inconsistent WinRM sub-frame metadata")
			}
			assembled = append(assembled, frame.Payload...)
			expectedFrameIndex++
			expectedSequence++
			if len(assembled) < logicalLength {
				if err := protocol.WriteAck(proc.Stdin, true); err != nil {
					return session, err
				}
				continue
			}
			if len(assembled) != logicalLength || expectedFrameIndex != expectedFrameCount {
				_ = protocol.WriteAck(proc.Stdin, false)
				return session, errors.New("logical chunk frame count or length mismatch")
			}
			logicalSum := sha256.Sum256(assembled)
			logicalHash := hex.EncodeToString(logicalSum[:])
			if !strings.EqualFold(logicalHash, assembledDigest) {
				_ = protocol.WriteAck(proc.Stdin, false)
				return session, fmt.Errorf("logical chunk SHA-256 mismatch at %d", assembledOffset)
			}
			base := "disk"
			if state.Kind == "ram" {
				base = "memory"
			}
			if replay {
				if err := evidence.VerifyExistingChunk(dir, base, state.SegmentSize, assembledOffset, len(assembled), logicalHash); err != nil {
					_ = protocol.WriteAck(proc.Stdin, false)
					return session, err
				}
			} else {
				if assembledOffset != state.NextOffset {
					_ = protocol.WriteAck(proc.Stdin, false)
					return session, fmt.Errorf("logical chunk offset changed: got %d want %d", assembledOffset, state.NextOffset)
				}
				if err := writer.WriteAt(assembled, assembledOffset); err != nil {
					_ = protocol.WriteAck(proc.Stdin, false)
					return session, err
				}
				rec := evidence.ChunkRecord{
					SessionID: h.SessionID, Sequence: assembledSequence, Offset: assembledOffset,
					Length: len(assembled), SHA256: logicalHash, ReadAt: assembledReadAt, WrittenAt: time.Now().UTC(),
				}
				if err := chunks.Append(rec); err != nil {
					_ = protocol.WriteAck(proc.Stdin, false)
					return session, err
				}
				_, _ = localSessionHash.Write(assembled)
				state.NextOffset += int64(len(assembled))
			}
			if err := evidence.SaveState(dir, state); err != nil {
				_ = protocol.WriteAck(proc.Stdin, false)
				return session, err
			}
			if err := protocol.WriteAck(proc.Stdin, true); err != nil {
				return session, err
			}
			printProgress(state, start, sessionStart)
			assembled = nil
			assembledOffset, assembledDigest, assembledReadAt = 0, "", time.Time{}
			expectedFrameIndex, expectedFrameCount = 0, 0
			replay = false
		case "trailer":
			session.Provider = h.Provider
			if assembled != nil {
				return session, errors.New("stream trailer arrived with an incomplete logical chunk")
			}
			if h.Sequence != expectedSequence {
				return session, fmt.Errorf("unexpected trailer sequence: got %d want %d", h.Sequence, expectedSequence)
			}
			_ = proc.Stdin.Close()
			waitErr := proc.Wait()
			<-stderrDone
			local := hex.EncodeToString(localSessionHash.Sum(nil))
			session.EndedAt, session.EndOffset = time.Now().UTC(), state.NextOffset
			session.RemoteSHA256, session.LocalSHA256 = h.StreamSHA256, local
			session.Bytes = state.NextOffset - start
			if waitErr != nil {
				session.Error = waitErr.Error()
				return session, fmt.Errorf("agent exit: %w: %s", waitErr, strings.TrimSpace(stderr.String()))
			}
			if h.Offset != state.NextOffset || h.Bytes != session.Bytes {
				session.Error = "trailer byte count mismatch"
				return session, errors.New(session.Error)
			}
			if !strings.EqualFold(local, h.StreamSHA256) {
				session.Error = "session stream SHA-256 mismatch"
				return session, errors.New(session.Error)
			}
			if src.Size > 0 && state.NextOffset != src.Size {
				session.Error = "source ended before declared size"
				return session, errors.New(session.Error)
			}
			if state.SessionCount == 1 {
				state.RemoteStreamHash = h.StreamSHA256
			}
			state.Providers = appendUnique(state.Providers, h.Provider)
			return session, nil
		case "error":
			return session, errors.New(h.Message)
		default:
			return session, fmt.Errorf("unexpected frame type %q", h.Type)
		}
	}
}

func (c *Controller) loadOrCreateState(dir, artifactID, kind string, src config.Source) (*evidence.State, error) {
	s, err := evidence.LoadState(dir)
	if err == nil {
		if s.CaseID != c.Job.Case.ID || s.ArtifactID != artifactID || s.Kind != kind ||
			s.SourceID != src.ID || (src.Size > 0 && s.SourceSize != src.Size) ||
			s.SourcePath != src.Path || s.SourceModel != src.Model ||
			(src.SectorSize > 0 && s.SectorSize != src.SectorSize) ||
			s.ChunkSize != c.Job.Acquisition.ChunkSize || s.SegmentSize != c.Job.Acquisition.SegmentSize {
			return nil, errors.New("resume state does not match requested case/source/layout")
		}
		return s, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	if len(entries) != 0 {
		return nil, errors.New("new artifact directory is not empty; refusing to mix prior evidence")
	}
	s = &evidence.State{
		Version: 1, CaseID: c.Job.Case.ID, EvidenceID: c.Job.Case.EvidenceID,
		ArtifactID: artifactID, Kind: kind, SourceID: src.ID, SourcePath: src.Path,
		SourceModel: src.Model,
		SourceSize:  src.Size, SectorSize: src.SectorSize, ChunkSize: c.Job.Acquisition.ChunkSize,
		SegmentSize: c.Job.Acquisition.SegmentSize, StartedAt: time.Now().UTC(),
		Status: evidence.StatusRunning,
	}
	if s.SourceID == "" && kind == "ram" {
		s.SourceID = "volatile-memory"
	}
	if kind == "ram" {
		s.ExpectedSize = c.Probe.MemoryBytes
	}
	if err := evidence.SaveState(dir, s); err != nil {
		return nil, err
	}
	return s, nil
}

func (c *Controller) Cleanup(ctx context.Context) error {
	if c.Job.Agent.KeepOnFailure {
		c.Footprint = append(c.Footprint, "Temporary remote files intentionally retained because agent.keep_on_failure is true")
		return nil
	}
	var errs []error
	if c.agentUploaded || len(c.uploadedTools) > 0 || c.markerPath != "" {
		if c.markerPath == "" || c.markerHash == "" {
			err := errors.New("temporary remote paths retained: case marker is unavailable")
			logErr := c.Events.Log(report.Event{Level: "error", Type: "cleanup", CaseID: c.Job.Case.ID, Message: err.Error()})
			c.Footprint = append(c.Footprint, err.Error())
			return errors.Join(err, logErr)
		}
		actual, err := c.remoteHash(ctx, c.markerPath)
		if err != nil || !strings.EqualFold(actual, c.markerHash) {
			if err == nil {
				err = errors.New("remote case marker hash mismatch")
			}
			err = fmt.Errorf("temporary remote paths retained: %w", err)
			logErr := c.Events.Log(report.Event{Level: "error", Type: "cleanup", CaseID: c.Job.Case.ID, Message: err.Error()})
			c.Footprint = append(c.Footprint, err.Error())
			return errors.Join(err, logErr)
		}
	}
	if c.AgentPath != "" {
		stdout, stderr, err := c.run(ctx, c.agentCommand("cleanup"))
		errs = append(errs, err)
		level, message := "info", "Agent footprint cleanup: "+strings.TrimSpace(stdout)
		if err != nil {
			level, message = "error", "Agent footprint cleanup failed: "+err.Error()+" "+strings.TrimSpace(stderr)
		}
		errs = append(errs, c.Events.Log(report.Event{Level: level, Type: "cleanup", CaseID: c.Job.Case.ID, Message: message}))
		c.Footprint = append(c.Footprint, message)
	}
	for i := len(c.uploadedTools) - 1; i >= 0; i-- {
		path := c.uploadedTools[i]
		err := c.Transport.Remove(ctx, path)
		errs = append(errs, err)
		level, message := "info", "Temporary tool removed: "+path
		if err != nil {
			level, message = "error", "Temporary tool cleanup failed: "+err.Error()
		}
		errs = append(errs, c.Events.Log(report.Event{Level: level, Type: "cleanup", CaseID: c.Job.Case.ID, Message: message}))
		c.Footprint = append(c.Footprint, message)
	}
	if c.agentUploaded {
		err := c.Transport.Remove(ctx, c.AgentPath)
		errs = append(errs, err)
		level, message := "info", "Temporary agent removed: "+c.AgentPath
		if err != nil {
			level, message = "error", "Temporary agent cleanup failed: "+err.Error()
		}
		errs = append(errs, c.Events.Log(report.Event{Level: level, Type: "cleanup", CaseID: c.Job.Case.ID, Message: message}))
		c.Footprint = append(c.Footprint, message)
	}
	for i := len(c.markerPaths) - 1; i >= 0; i-- {
		path := c.markerPaths[i]
		if err := c.Transport.Remove(ctx, path); err != nil {
			errs = append(errs, err)
			errs = append(errs, c.Events.Log(report.Event{Level: "error", Type: "cleanup", CaseID: c.Job.Case.ID, Message: "Remote case marker cleanup failed: " + err.Error()}))
			c.Footprint = append(c.Footprint, "Remote case marker cleanup failed: "+path)
		} else {
			errs = append(errs, c.Events.Log(report.Event{Level: "info", Type: "cleanup", CaseID: c.Job.Case.ID, Message: "Remote case marker removed: " + path}))
			c.Footprint = append(c.Footprint, "Remote case marker removed: "+path)
		}
	}
	return errors.Join(errs...)
}

func (c *Controller) installRemoteMarker(ctx context.Context) error {
	marker := evidence.RemoteMarker{
		Version: 1, CaseID: c.Job.Case.ID, EvidenceID: c.Job.Case.EvidenceID,
		AgentPath: c.AgentPath, RemoveAgent: c.agentUploaded, ToolPaths: append([]string(nil), c.uploadedTools...),
		PriorMarkers: append([]string(nil), c.markerPaths...), CreatedAt: time.Now().UTC(),
	}
	raw, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	localCandidate := filepath.Join(c.CaseDir, ".remote-footprint-marker.candidate.json")
	if err := evidence.AtomicWrite(localCandidate, raw, 0o600); err != nil {
		return err
	}
	defer os.Remove(localCandidate)
	remote := remoteJoin(remoteParent(c.AgentPath, c.Transport.Name()), ".imajer-case-marker-"+hash[:16]+".json", c.Transport.Name())
	if err := c.Transport.Upload(ctx, localCandidate, remote, 0o600); err != nil {
		return err
	}
	actual, err := c.remoteHash(ctx, remote)
	if err != nil || !strings.EqualFold(actual, hash) {
		_ = c.Transport.Remove(ctx, remote)
		if err != nil {
			return err
		}
		return errors.New("remote case marker hash mismatch after upload")
	}
	if err := evidence.AtomicWrite(filepath.Join(c.CaseDir, "remote-footprint-marker.json"), raw, 0o600); err != nil {
		_ = c.Transport.Remove(ctx, remote)
		return err
	}
	old := c.markerPath
	c.markerPath, c.markerHash = remote, hash
	c.markerPaths = appendUnique(c.markerPaths, remote)
	if old != "" && old != remote {
		_ = c.Transport.Remove(ctx, old)
	}
	return nil
}

func (c *Controller) remoteHash(ctx context.Context, path string) (string, error) {
	sum, err := c.Transport.HashFile(ctx, path)
	if err != nil {
		return "", err
	}
	if len(sum) != 64 {
		return "", errors.New("invalid remote SHA-256 output")
	}
	return sum, nil
}

func (c *Controller) agentCommand(args ...string) []string {
	command := append([]string{c.AgentPath}, args...)
	if c.useSudo {
		command = append([]string{"sudo", "-n"}, command...)
	}
	return command
}

func (c *Controller) run(ctx context.Context, argv []string) (string, string, error) {
	s, err := c.Transport.Start(ctx, argv)
	if err != nil {
		return "", "", err
	}
	_ = s.Stdin.Close()
	var stdout, stderr []byte
	var outErr, errErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); stdout, outErr = io.ReadAll(io.LimitReader(s.Stdout, 16<<20)) }()
	go func() { defer wg.Done(); stderr, errErr = io.ReadAll(io.LimitReader(s.Stderr, 4<<20)) }()
	waitErr := s.Wait()
	wg.Wait()
	return string(stdout), string(stderr), errors.Join(waitErr, outErr, errErr)
}

func matchingAgentArtifact(m *tools.Manifest, path string) (*tools.Artifact, error) {
	for i := range m.Artifacts {
		a := &m.Artifacts[i]
		if a.Name != "imajer-agent" {
			continue
		}
		if err := tools.VerifyFile(path, a.SHA256); err == nil {
			return a, nil
		}
	}
	return nil, errors.New("local agent is not present in signed manifest")
}

func matchingRemoteAgentArtifact(m *tools.Manifest, digest, transportName string) (*tools.Artifact, error) {
	expectedOS := "linux"
	if transportName == "winrm" {
		expectedOS = "windows"
	}
	for i := range m.Artifacts {
		artifact := &m.Artifacts[i]
		if artifact.Name == "imajer-agent" && artifact.OS == expectedOS &&
			strings.EqualFold(artifact.SHA256, digest) {
			return artifact, nil
		}
	}
	return nil, fmt.Errorf("preinstalled agent SHA-256 is not present in signed manifest for %s", expectedOS)
}

func nextRAMAttempt(caseDir string) int {
	root := filepath.Join(caseDir, "artifacts")
	entries, _ := os.ReadDir(root)
	maxN := 0
	for _, e := range entries {
		var n int
		if _, err := fmt.Sscanf(e.Name(), "memory-attempt-%03d", &n); err == nil && n > maxN {
			maxN = n
		}
	}
	return maxN + 1
}

func defaultProvider(s string) string {
	if s == "" {
		return "auto"
	}
	return s
}

func remoteParent(path, transportName string) string {
	sep := "/"
	if transportName == "winrm" {
		sep = `\`
	}
	i := strings.LastIndex(path, sep)
	if i <= 0 {
		return "."
	}
	return path[:i]
}

func remoteJoin(parent, name, transportName string) string {
	sep := "/"
	if transportName == "winrm" {
		sep = `\`
	}
	return strings.TrimRight(parent, sep) + sep + name
}

func appendUnique(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func readFrameWithTimeout(decoder *protocol.Decoder, timeout time.Duration) (*protocol.Frame, error) {
	if timeout <= 0 {
		return decoder.Read()
	}
	type result struct {
		frame *protocol.Frame
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		frame, err := decoder.Read()
		ch <- result{frame: frame, err: err}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case result := <-ch:
		return result.frame, result.err
	case <-timer.C:
		return nil, fmt.Errorf("chunk read timeout after %s", timeout)
	}
}

func backoff(attempt int) time.Duration {
	d := time.Second << min(attempt-1, 5)
	if d > 30*time.Second {
		return 30 * time.Second
	}
	return d
}

func printProgress(state *evidence.State, sessionOffset int64, started time.Time) {
	sessionElapsed := time.Since(started).Seconds()
	if sessionElapsed < 0.001 {
		sessionElapsed = 0.001
	}
	currentSpeed := float64(state.NextOffset-sessionOffset) / sessionElapsed
	totalElapsed := time.Since(state.StartedAt).Seconds()
	if totalElapsed < 0.001 {
		totalElapsed = 0.001
	}
	averageSpeed := float64(state.NextOffset) / totalElapsed
	var pct, eta string
	total := state.SourceSize
	if total == 0 {
		total = state.ExpectedSize
	}
	if total > 0 {
		pctValue := min(100, 100*float64(state.NextOffset)/float64(total))
		pct = fmt.Sprintf("%6.2f%%", pctValue)
		if averageSpeed > 0 {
			remaining := max(int64(0), total-state.NextOffset)
			eta = (time.Duration(float64(remaining)/averageSpeed) * time.Second).Round(time.Second).String()
		}
	} else {
		pct, eta = "  n/a ", "n/a"
	}
	fmt.Fprintf(os.Stderr, "\r%s %s  %s / %s  anlik %.1f MiB/s  ort %.1f MiB/s  ETA %s  retry=%d  offset=%d  verify=chunk-sha256",
		state.ArtifactID, pct, humanBytes(state.NextOffset), humanBytes(total),
		currentSpeed/(1<<20), averageSpeed/(1<<20), eta, state.RetryCount, state.NextOffset)
}

func humanBytes(n int64) string {
	if n <= 0 {
		return "0 B"
	}
	const unit = 1024
	exp, div := 0, int64(unit)
	for v := n / unit; v >= unit && exp < 5; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func randomID(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

type limitedWriter struct {
	W io.Writer
	N int64
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if w.N <= 0 {
		return len(p), nil
	}
	q := p
	if int64(len(q)) > w.N {
		q = q[:w.N]
	}
	n, err := w.W.Write(q)
	w.N -= int64(n)
	if err != nil {
		return n, err
	}
	return len(p), nil
}
