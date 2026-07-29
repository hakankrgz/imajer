package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/hakankrgz/imajer/internal/config"
	"github.com/hakankrgz/imajer/internal/controller"
	"github.com/hakankrgz/imajer/internal/evidence"
	"github.com/hakankrgz/imajer/internal/fsutil"
	"github.com/hakankrgz/imajer/internal/probe"
	"github.com/hakankrgz/imajer/internal/report"
	"github.com/hakankrgz/imajer/internal/tools"
	"github.com/hakankrgz/imajer/internal/transport"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

var (
	version           = "0.6.3"
	desktopMode       = "false"
	desktopWindowMode = "false"
)

const cancellationFileEnv = "IMAJER_CANCELLATION_FILE"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	watchCancellationFile(ctx, cancel, os.Getenv(cancellationFileEnv))
	if len(os.Args) < 2 {
		if desktopMode == "true" {
			if err := runUI(ctx, nil); err != nil {
				fmt.Fprintln(os.Stderr, "imajer:", err)
				os.Exit(1)
			}
			return
		}
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "acquire", "resume":
		err = acquire(ctx, os.Args[2:])
	case "discover":
		err = discover(ctx, os.Args[2:])
	case "verify":
		err = verify(os.Args[2:])
	case "report":
		err = regenerateReport(os.Args[2:])
	case "cleanup":
		err = cleanup(ctx, os.Args[2:])
	case "ui":
		err = runUI(ctx, os.Args[2:])
	case "wizard":
		err = wizard(os.Args[2:])
	case "tools":
		err = toolCommand(os.Args[2:])
	case "version":
		fmt.Println(version)
	default:
		usage()
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "imajer:", err)
		os.Exit(1)
	}
}

func watchCancellationFile(ctx context.Context, cancel context.CancelFunc, path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				info, err := os.Lstat(path)
				if err == nil && info.Mode().IsRegular() {
					cancel()
					return
				}
			}
		}
	}()
}

func acquire(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("acquire", flag.ContinueOnError)
	jobPath := fs.String("job", "", "YAML job file")
	profile := fs.String("profile", "", "override profile: ram, disk or both")
	outputDir := fs.String("output", "", "override local evidence root")
	signingKey := fs.String("signing-key", "", "override PKCS#8 Ed25519 signing key")
	targetHost := fs.String("host", "", "override target host")
	targetPort := fs.Int("port", 0, "override target port")
	targetUser := fs.String("user", "", "override target user")
	passwordEnv := fs.String("password-env", "", "override credential environment variable name")
	diskPath := fs.String("disk-path", "", "override physical disk path")
	diskID := fs.String("disk-id", "", "override stable physical disk ID")
	diskModel := fs.String("disk-model", "", "override physical disk model")
	diskSize := fs.Int64("disk-size", 0, "override physical disk size in bytes")
	diskSectorSize := fs.Int64("disk-sector-size", 0, "override physical sector size in bytes")
	diskProvider := fs.String("disk-provider", "", "override disk provider")
	ramProvider := fs.String("ram-provider", "", "override RAM provider")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *jobPath == "" {
		return errors.New("--job is required; use 'imajer wizard' to create one")
	}
	job, err := config.LoadForOverrides(*jobPath)
	if err != nil {
		return err
	}
	if *profile != "" {
		job.Acquisition.Profile = *profile
	}
	if *outputDir != "" {
		job.Output.Directory = *outputDir
	}
	if *signingKey != "" {
		job.Output.SigningKey = *signingKey
	}
	if *targetHost != "" {
		job.Target.Host = *targetHost
	}
	if *targetPort != 0 {
		job.Target.Port = *targetPort
	}
	if *targetUser != "" {
		job.Target.User = *targetUser
	}
	if *passwordEnv != "" {
		job.Target.PasswordEnv = *passwordEnv
	}
	if *diskPath != "" {
		job.Acquisition.Disk.Path = *diskPath
	}
	if *diskID != "" {
		job.Acquisition.Disk.ID = *diskID
	}
	if *diskModel != "" {
		job.Acquisition.Disk.Model = *diskModel
	}
	if *diskSize != 0 {
		job.Acquisition.Disk.Size = *diskSize
	}
	if *diskSectorSize != 0 {
		job.Acquisition.Disk.SectorSize = *diskSectorSize
	}
	if *diskProvider != "" {
		job.Acquisition.Disk.Provider = *diskProvider
	}
	if *ramProvider != "" {
		job.Acquisition.RAM.Provider = *ramProvider
	}
	if err := job.Validate(); err != nil {
		return fmt.Errorf("job after flag overrides: %w", err)
	}
	if err := promptCredentialIfNeeded(job); err != nil {
		return err
	}
	if job.Output.SigningKey == "" {
		return errors.New("output.signing_key is required for an evidentiary acquisition")
	}
	c, err := controller.New(ctx, job)
	if err != nil {
		return err
	}
	states, acquireErr := c.Acquire(ctx)
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), job.Retry.Cleanup)
	cleanupErr := c.Cleanup(cleanupCtx)
	cleanupCancel()
	probeInfo := c.Probe
	closeErr := c.Close()

	allStates, loadErr := loadAllStates(c.CaseDir)
	if loadErr == nil {
		states = allStates
		for i := range states {
			if err := controller.FinalizeNonCompleteArtifact(c.CaseDir, &states[i]); err != nil {
				loadErr = errors.Join(loadErr, fmt.Errorf("finalize artifact %s: %w", states[i].ArtifactID, err))
			}
		}
	} else if errors.Is(loadErr, os.ErrNotExist) {
		loadErr = nil
	}
	sessions, sessionsErr := loadAllSessions(c.CaseDir, states)
	loadErr = errors.Join(loadErr, sessionsErr)
	footprint := []string{
		"Hedefte RAM veya disk imaj/staging dosyasi olusturulmadi.",
		"Gecici agent/arac dosyalari, surucu/modul ve OS audit kayitlari hedef footprint'ine dahildir.",
	}
	footprint = append(footprint, c.Footprint...)
	warnings := append([]string(nil), probeInfo.Warnings...)
	if job.Acquisition.Profile == "disk" || job.Acquisition.Profile == "both" {
		warnings = append(warnings, "Canli fiziksel disk imaji tek atomik zamani temsil etmez.")
	}
	if acquireErr != nil {
		warnings = append(warnings, "Edinim tamamlanmadi: "+acquireErr.Error())
	}
	if cleanupErr != nil {
		warnings = append(warnings, "Hedef footprint tamamen temizlenemedi: "+cleanupErr.Error())
	}
	data := report.CaseReport{
		Case: job.Case, Profile: job.Acquisition.Profile, Target: probeInfo,
		LocalStorage: c.LocalStorage, Artifacts: states, Sessions: sessions, Tools: c.Tools,
		Footprint: footprint, Warnings: warnings,
	}
	reportErr := report.WriteCaseReport(c.CaseDir, data)
	var indexErr error
	if reportErr == nil {
		_, indexErr = report.FinalizeIndex(c.CaseDir, job.Case.ID, job.Case.EvidenceID, job.Output.SigningKey)
	}
	return errors.Join(acquireErr, cleanupErr, closeErr, loadErr, reportErr, indexErr)
}

func discover(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("discover", flag.ContinueOnError)
	jobPath := fs.String("job", "", "YAML job file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *jobPath == "" {
		return errors.New("--job is required")
	}
	job, err := config.Load(*jobPath)
	if err != nil {
		return err
	}
	if err := promptCredentialIfNeeded(job); err != nil {
		return err
	}
	c, err := controller.New(ctx, job)
	if err != nil {
		return err
	}
	if err := c.Prepare(ctx); err != nil {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), job.Retry.Cleanup)
		_ = c.Cleanup(cleanupCtx)
		cleanupCancel()
		_ = c.Close()
		return err
	}
	raw, _ := json.MarshalIndent(c.Probe, "", "  ")
	fmt.Println(string(raw))
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), job.Retry.Cleanup)
	cleanupErr := c.Cleanup(cleanupCtx)
	cleanupCancel()
	return errors.Join(cleanupErr, c.Close())
}

func verify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	caseDir := fs.String("case-dir", "", "case evidence directory")
	publicKey := fs.String("public-key", "", "independently trusted examiner Ed25519 public key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *caseDir == "" {
		return errors.New("--case-dir is required")
	}
	// Authenticate the package before parsing state fields that influence local
	// paths, allocation sizes or verification loops.
	if err := report.VerifyIndex(*caseDir, *publicKey); err != nil {
		return err
	}
	states, err := loadAllStates(*caseDir)
	if err != nil {
		return err
	}
	verifiedCount, partialCount := 0, 0
	var failed []string
	for _, state := range states {
		dir := filepath.Join(*caseDir, "artifacts", state.ArtifactID)
		base := "disk"
		if state.Kind == "ram" {
			base = "memory"
		}
		summary, err := evidence.AuditChunks(dir, base, state.SegmentSize)
		if err != nil {
			return err
		}
		if summary.EndOffset != state.NextOffset {
			return fmt.Errorf("artifact %s journal ends at %d but state says %d", state.ArtifactID, summary.EndOffset, state.NextOffset)
		}
		if state.Status == evidence.StatusVerifiedContinuous || state.Status == evidence.StatusChunkVerifiedComposite {
			logical, _, err := evidence.HashLogical(dir, base, state.SegmentSize, state.NextOffset)
			if err != nil {
				return err
			}
			if !strings.EqualFold(logical, state.LogicalSHA256) || !strings.EqualFold(summary.MerkleRoot, state.MerkleRoot) {
				return fmt.Errorf("artifact %s logical hash or Merkle root mismatch", state.ArtifactID)
			}
			verifiedCount++
			fmt.Printf("ACQUISITION_VERIFIED: %s (%s)\n", state.ArtifactID, state.Status)
		} else if state.Status == evidence.StatusIncomplete {
			partialCount++
			fmt.Printf("NON_EVIDENTIARY_PARTIAL: %s (%d bytes)\n", state.ArtifactID, state.NextOffset)
		} else {
			failed = append(failed, state.ArtifactID+"="+string(state.Status))
		}
	}
	fmt.Printf("PACKAGE_INTEGRITY_OK: signed evidence index valid; verified=%d partial=%d\n", verifiedCount, partialCount)
	if len(failed) > 0 || verifiedCount == 0 {
		return fmt.Errorf("acquisition is not complete: failed=%v verified=%d partial=%d", failed, verifiedCount, partialCount)
	}
	return nil
}

func regenerateReport(args []string) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	jobPath := fs.String("job", "", "YAML job file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *jobPath == "" {
		return errors.New("--job is required")
	}
	job, err := config.Load(*jobPath)
	if err != nil {
		return err
	}
	caseDir := filepath.Join(job.Output.Directory, job.Case.ID, job.Case.EvidenceID)
	states, err := loadAllStates(caseDir)
	if err != nil {
		return err
	}
	var info probe.Info
	raw, err := os.ReadFile(filepath.Join(caseDir, "target-probe.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &info); err != nil {
		return err
	}
	var data report.CaseReport
	if previous, readErr := os.ReadFile(filepath.Join(caseDir, "case-report.json")); readErr == nil {
		_ = json.Unmarshal(previous, &data)
	}
	sessions, err := loadAllSessions(caseDir, states)
	if err != nil {
		return err
	}
	data.Case = job.Case
	data.Profile = job.Acquisition.Profile
	data.Target = info
	data.Artifacts = states
	data.Sessions = sessions
	if len(data.Footprint) == 0 {
		data.Footprint = []string{"Zero disk footprint yalniz imaj/staging verisini kapsar."}
	}
	if len(data.Warnings) == 0 {
		data.Warnings = []string{"Canli kaynak ediniminin zamansal sinirlari artifact manifestlerinde kayitlidir."}
	}
	if storage, inspectErr := fsutil.Inspect(caseDir); inspectErr == nil {
		data.LocalStorage = storage
	}
	if err := report.WriteCaseReport(caseDir, data); err != nil {
		return err
	}
	_, err = report.FinalizeIndex(caseDir, job.Case.ID, job.Case.EvidenceID, job.Output.SigningKey)
	return err
}

func cleanup(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("cleanup", flag.ContinueOnError)
	jobPath := fs.String("job", "", "YAML job file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *jobPath == "" {
		return errors.New("--job is required")
	}
	job, err := config.Load(*jobPath)
	if err != nil {
		return err
	}
	if err := promptCredentialIfNeeded(job); err != nil {
		return err
	}
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), job.Retry.Cleanup)
	defer cleanupCancel()
	ctx = cleanupCtx
	caseDir := filepath.Join(job.Output.Directory, job.Case.ID, job.Case.EvidenceID)
	markerRaw, err := os.ReadFile(filepath.Join(caseDir, "remote-footprint-marker.json"))
	if err != nil {
		return fmt.Errorf("read locally retained remote case marker: %w", err)
	}
	var marker evidence.RemoteMarker
	if err := json.Unmarshal(markerRaw, &marker); err != nil {
		return fmt.Errorf("decode remote case marker: %w", err)
	}
	if marker.Version != 1 || marker.CaseID != job.Case.ID || marker.EvidenceID != job.Case.EvidenceID {
		return errors.New("remote case marker does not match the requested case")
	}
	path := marker.AgentPath
	if path == "" || !strings.Contains(strings.ToLower(path), "imajer-") {
		return errors.New("remote case marker contains an unsafe agent path")
	}
	if job.Agent.RemotePath != "" && job.Agent.RemotePath != path {
		return errors.New("job agent.remote_path does not match the retained case marker")
	}
	tr, err := transport.New(ctx, job.Target, job.Retry.Connect)
	if err != nil {
		return err
	}
	defer tr.Close()
	sum := sha256.Sum256(markerRaw)
	expectedMarkerHash := hex.EncodeToString(sum[:])
	remoteMarker := derivedRemoteMarkerPath(path, tr.Name(), expectedMarkerHash)
	actualMarkerHash, err := tr.HashFile(ctx, remoteMarker)
	if err != nil {
		return fmt.Errorf("verify remote case marker: %w", err)
	}
	if !strings.EqualFold(actualMarkerHash, expectedMarkerHash) {
		return errors.New("remote case marker hash mismatch; no path was removed")
	}
	session, startErr := tr.Start(ctx, []string{path, "cleanup"})
	var cleanupErr error
	if startErr == nil {
		_ = session.Stdin.Close()
		var stdoutErr, stderrErr error
		done := make(chan struct{}, 2)
		go func() { _, stdoutErr = io.Copy(io.Discard, session.Stdout); done <- struct{}{} }()
		go func() { _, stderrErr = io.Copy(io.Discard, session.Stderr); done <- struct{}{} }()
		cleanupErr = session.Wait()
		<-done
		<-done
		cleanupErr = errors.Join(cleanupErr, stdoutErr, stderrErr)
	} else {
		cleanupErr = startErr
	}
	var removeErrs []error
	for i := len(marker.ToolPaths) - 1; i >= 0; i-- {
		removeErrs = append(removeErrs, tr.Remove(ctx, marker.ToolPaths[i]))
	}
	if marker.RemoveAgent {
		removeErrs = append(removeErrs, tr.Remove(ctx, path))
	}
	for i := len(marker.PriorMarkers) - 1; i >= 0; i-- {
		removeErrs = append(removeErrs, tr.Remove(ctx, marker.PriorMarkers[i]))
	}
	removeErrs = append(removeErrs, tr.Remove(ctx, remoteMarker))
	return errors.Join(cleanupErr, errors.Join(removeErrs...))
}

func derivedRemoteMarkerPath(agentPath, transportName, hash string) string {
	separator := "/"
	if transportName == "winrm" {
		separator = `\`
	}
	i := strings.LastIndex(agentPath, separator)
	parent := "."
	if i > 0 {
		parent = agentPath[:i]
	}
	return strings.TrimRight(parent, separator) + separator + ".imajer-case-marker-" + hash[:16] + ".json"
}

func wizard(args []string) error {
	fs := flag.NewFlagSet("wizard", flag.ContinueOnError)
	outPath := fs.String("out", "", "write YAML to this file; otherwise stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	in := bufio.NewReader(os.Stdin)
	ask := func(label string) string {
		fmt.Fprint(os.Stderr, label+": ")
		s, _ := in.ReadString('\n')
		return strings.TrimSpace(s)
	}
	var j config.Job
	j.Case.ID = ask("Vaka ID")
	j.Case.EvidenceID = ask("Delil ID")
	j.Case.Examiner = ask("Incelemeci")
	j.Case.Organization = ask("Kurum")
	j.Case.AuthorityRef = ask("Yetki referansi")
	j.Case.Authorized = strings.EqualFold(ask("Yasal yetki onayi (EVET)"), "EVET")
	j.Target.Transport = strings.ToLower(ask("Transport (ssh/winrm/local)"))
	j.Target.Host = ask("Hedef host")
	j.Target.User = ask("Kullanici")
	j.Target.PasswordEnv = ask("Parola/passphrase ortam degiskeni adi (opsiyonel)")
	j.Target.KnownHosts = ask("known_hosts yolu (SSH)")
	j.Target.CAFile = ask("CA PEM yolu (WinRM)")
	j.Acquisition.Profile = strings.ToLower(ask("Profil (ram/disk/both)"))
	if j.Acquisition.Profile == "disk" || j.Acquisition.Profile == "both" {
		j.Acquisition.Disk.Path = ask("Fiziksel disk yolu")
		j.Acquisition.Disk.ID = ask("Disk stabil ID/seri")
		j.Acquisition.Disk.Model = ask("Disk model")
		fmt.Sscan(ask("Disk boyutu (byte)"), &j.Acquisition.Disk.Size)
		fmt.Sscan(ask("Disk sektor boyutu (byte, genellikle 512/4096)"), &j.Acquisition.Disk.SectorSize)
	}
	j.Acquisition.RAM.ID = "volatile-memory"
	j.Acquisition.RAM.Provider = "auto"
	j.Output.Directory = ask("Yerel cikti dizini")
	j.Output.SigningKey = ask("PKCS#8 Ed25519 private key yolu")
	j.Agent.LocalPath = ask("Yerel imajer-agent binary yolu")
	j.Agent.ToolManifest = ask("Imzali tool manifest yolu")
	j.Agent.TrustPublicKey = ask("Tool trust public key yolu")
	raw, err := yaml.Marshal(j)
	if err != nil {
		return err
	}
	if *outPath == "" {
		fmt.Print(string(raw))
		return nil
	}
	return os.WriteFile(*outPath, raw, 0o600)
}

func promptCredentialIfNeeded(job *config.Job) error {
	if config.Password(job.Target) != "" {
		return nil
	}
	needsPassword := false
	switch strings.ToLower(job.Target.Transport) {
	case "ssh":
		if os.Getenv("SSH_AUTH_SOCK") == "" {
			if job.Target.PrivateKey == "" {
				needsPassword = true
			} else {
				raw, err := os.ReadFile(job.Target.PrivateKey)
				if err != nil {
					return err
				}
				_, parseErr := ssh.ParsePrivateKey(raw)
				var missing *ssh.PassphraseMissingError
				needsPassword = errors.As(parseErr, &missing)
			}
		}
	case "winrm":
		needsPassword = !strings.EqualFold(job.Target.Auth, "kerberos") || job.Target.KerberosCCache == ""
	}
	if !needsPassword || !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil
	}
	fmt.Fprint(os.Stderr, "Hedef parola/private-key passphrase: ")
	raw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return fmt.Errorf("read credential: %w", err)
	}
	if len(raw) == 0 {
		return errors.New("empty credential is not accepted")
	}
	job.Target.RuntimePassword = string(raw)
	return nil
}

func toolCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: imajer tools <verify|sign>")
	}
	if args[0] == "sign" {
		fs := flag.NewFlagSet("tools sign", flag.ContinueOnError)
		specPath := fs.String("spec", "", "YAML artifact specification")
		keyPath := fs.String("key", "", "PKCS#8 Ed25519 private key")
		outPath := fs.String("out", "", "output manifest")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		raw, err := os.ReadFile(*specPath)
		if err != nil {
			return err
		}
		var artifacts []tools.Artifact
		if err := yaml.Unmarshal(raw, &artifacts); err != nil {
			return err
		}
		return tools.Create(artifacts, *keyPath, *outPath)
	}
	if args[0] != "verify" {
		return errors.New("usage: imajer tools <verify|sign>")
	}
	fs := flag.NewFlagSet("tools verify", flag.ContinueOnError)
	manifestPath := fs.String("manifest", "", "signed manifest")
	keyPath := fs.String("key", "", "Ed25519 public key")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	m, err := tools.LoadAndVerify(*manifestPath, *keyPath)
	if err != nil {
		return err
	}
	base := filepath.Dir(*manifestPath)
	for _, a := range m.Artifacts {
		if err := tools.VerifyFile(filepath.Join(base, a.Path), a.SHA256); err != nil {
			return err
		}
	}
	fmt.Printf("VERIFIED: signed manifest and %d artifact(s)\n", len(m.Artifacts))
	return nil
}

func loadAllStates(caseDir string) ([]evidence.State, error) {
	root := filepath.Join(caseDir, "artifacts")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var states []evidence.State
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		s, err := evidence.LoadState(filepath.Join(root, e.Name()))
		if err != nil {
			return nil, err
		}
		if err := validateArtifactState(*s, e.Name()); err != nil {
			return nil, fmt.Errorf("invalid artifact state in %s: %w", e.Name(), err)
		}
		states = append(states, *s)
	}
	return states, nil
}

func validateArtifactState(state evidence.State, directoryName string) error {
	if state.ArtifactID != directoryName || !safeEvidenceComponent(state.ArtifactID) {
		return errors.New("artifact_id must match its directory and contain no path separators")
	}
	if !safeEvidenceComponent(state.CaseID) || !safeEvidenceComponent(state.EvidenceID) {
		return errors.New("case_id and evidence_id are invalid")
	}
	if state.Kind != "disk" && state.Kind != "ram" {
		return fmt.Errorf("unsupported artifact kind %q", state.Kind)
	}
	switch state.Status {
	case evidence.StatusRunning, evidence.StatusVerifiedContinuous,
		evidence.StatusChunkVerifiedComposite, evidence.StatusIncomplete, evidence.StatusFailed:
	default:
		return fmt.Errorf("unsupported artifact status %q", state.Status)
	}
	if state.ChunkSize < 1<<20 || state.ChunkSize > 64<<20 {
		return errors.New("chunk_size must be between 1 MiB and 64 MiB")
	}
	if state.SegmentSize < state.ChunkSize || state.SegmentSize > 4<<30 ||
		state.SegmentSize%state.ChunkSize != 0 {
		return errors.New("segment_size is invalid")
	}
	if state.NextOffset < 0 || state.SourceSize < 0 || state.ExpectedSize < 0 ||
		state.SessionCount < 0 || state.RetryCount < 0 {
		return errors.New("state contains a negative size or counter")
	}
	if state.SourceSize > 0 && state.NextOffset > state.SourceSize {
		return errors.New("next_offset exceeds source_size")
	}
	if state.Status == evidence.StatusVerifiedContinuous ||
		state.Status == evidence.StatusChunkVerifiedComposite {
		if !validSHA256(state.LogicalSHA256) || !validSHA256(state.MerkleRoot) {
			return errors.New("verified artifact is missing a valid logical hash or Merkle root")
		}
	}
	return nil
}

func safeEvidenceComponent(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	return filepath.Base(value) == value && !strings.ContainsAny(value, `/\`+"\x00\r\n")
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func loadAllSessions(caseDir string, states []evidence.State) (map[string][]evidence.SessionRecord, error) {
	result := make(map[string][]evidence.SessionRecord, len(states))
	for _, state := range states {
		records, err := evidence.ReadSessions(filepath.Join(caseDir, "artifacts", state.ArtifactID))
		if err != nil {
			return nil, fmt.Errorf("read sessions for %s: %w", state.ArtifactID, err)
		}
		result[state.ArtifactID] = records
	}
	return result, nil
}

func usage() {
	fmt.Fprintln(os.Stderr, `imajer - in-memory remote forensic acquisition

Commands:
  discover --job job.yaml
  acquire  --job job.yaml [--profile ... --output ... --host ... --disk-* ...]
  resume   --job job.yaml [acquire override flags]
  verify   --case-dir PATH [--public-key examiner-public.pem]
  report   --job job.yaml
  cleanup  --job job.yaml
  ui       [--listen 127.0.0.1:8765] [--no-open]
  wizard   [--out job.yaml]
  tools verify --manifest tools.json --key trust.pem
  tools sign --spec tools.yaml --key release-private.pem --out tool-manifest.json
  version`)
}
