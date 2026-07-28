package main

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"embed"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hakankrgz/imajer/internal/config"
	"github.com/hakankrgz/imajer/internal/evidence"
	"gopkg.in/yaml.v3"
)

//go:embed ui/*
var uiFiles embed.FS

type uiServer struct {
	mu                sync.Mutex
	token             string
	executable        string
	workingDir        string
	resourceDir       string
	defaultSigningKey string
	packaged          bool
	cancel            context.CancelFunc
	shutdown          chan struct{}
	status            uiStatus
}

type uiStatus struct {
	Running    bool      `json:"running"`
	Action     string    `json:"action,omitempty"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	Success    bool      `json:"success"`
	Message    string    `json:"message,omitempty"`
	JobPath    string    `json:"job_path,omitempty"`
	CaseDir    string    `json:"case_dir,omitempty"`
	Logs       []string  `json:"logs"`
}

type uiRunRequest struct {
	Action    string     `json:"action"`
	JobPath   string     `json:"job_path,omitempty"`
	CaseDir   string     `json:"case_dir,omitempty"`
	PublicKey string     `json:"public_key,omitempty"`
	Password  string     `json:"password,omitempty"`
	Job       *uiJobForm `json:"job,omitempty"`
}

type uiJobForm struct {
	CaseID          string `json:"case_id"`
	EvidenceID      string `json:"evidence_id"`
	Examiner        string `json:"examiner"`
	Organization    string `json:"organization"`
	AuthorityRef    string `json:"authority_ref"`
	Notes           string `json:"notes"`
	Authorized      bool   `json:"authorized"`
	Transport       string `json:"transport"`
	Host            string `json:"host"`
	Port            string `json:"port"`
	User            string `json:"user"`
	Auth            string `json:"auth"`
	PrivateKey      string `json:"private_key"`
	KnownHosts      string `json:"known_hosts"`
	CAFile          string `json:"ca_file"`
	Profile         string `json:"profile"`
	DiskPath        string `json:"disk_path"`
	DiskID          string `json:"disk_id"`
	DiskModel       string `json:"disk_model"`
	DiskSize        string `json:"disk_size"`
	SectorSize      string `json:"sector_size"`
	DiskProvider    string `json:"disk_provider"`
	RAMProvider     string `json:"ram_provider"`
	RAMToolName     string `json:"ram_tool_name"`
	RAMToolLocal    string `json:"ram_tool_local"`
	OutputDirectory string `json:"output_directory"`
	SigningKey      string `json:"signing_key"`
	AgentLocal      string `json:"agent_local"`
	AgentRemote     string `json:"agent_remote"`
	ToolManifest    string `json:"tool_manifest"`
	TrustPublicKey  string `json:"trust_public_key"`
}

func runUI(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("ui", flag.ContinueOnError)
	listen := flags.String("listen", "127.0.0.1:8765", "localhost listen address")
	noOpen := flags.Bool("no-open", false, "do not open the default browser")
	if err := flags.Parse(args); err != nil {
		return err
	}
	host, _, err := net.SplitHostPort(*listen)
	if err != nil {
		return fmt.Errorf("invalid listen address: %w", err)
	}
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return errors.New("UI may listen only on localhost")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	workingDir, resourceDir, packaged, err := resolveUIRuntime(executable)
	if err != nil {
		return err
	}
	defaultSigningKey := ""
	if packaged {
		defaultSigningKey, _, err = ensureExaminerKey(workingDir)
		if err != nil {
			return fmt.Errorf("incelemeci imza anahtarını hazırla: %w", err)
		}
	}
	token, err := secureToken()
	if err != nil {
		return err
	}
	server := &uiServer{
		token: token, executable: executable, workingDir: workingDir,
		resourceDir: resourceDir, defaultSigningKey: defaultSigningKey,
		packaged: packaged, shutdown: make(chan struct{}, 1),
		status: uiStatus{Message: "Hazır", Logs: []string{}},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", server.handleHealth)
	mux.HandleFunc("/api/config", server.handleConfig)
	mux.HandleFunc("/api/status", server.handleStatus)
	mux.HandleFunc("/api/run", server.requireToken(server.handleRun))
	mux.HandleFunc("/api/cancel", server.requireToken(server.handleCancel))
	mux.HandleFunc("/api/shutdown", server.requireToken(server.handleShutdown))
	assets, err := fs.Sub(uiFiles, "ui")
	if err != nil {
		return err
	}
	mux.Handle("/", http.FileServer(http.FS(assets)))
	httpServer := &http.Server{
		Addr:              *listen,
		Handler:           localhostOnly(securityHeaders(mux)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		if openRunningUI(*listen) {
			return nil
		}
		return err
	}
	url := "http://" + listener.Addr().String()
	fmt.Println("IMAJER arayüzü:", url)
	fmt.Println("Kapatmak için Ctrl+C")
	if !*noOpen {
		go func() {
			time.Sleep(250 * time.Millisecond)
			_ = openBrowser(url)
		}()
	}
	go func() {
		select {
		case <-ctx.Done():
		case <-server.shutdown:
		}
		server.mu.Lock()
		if server.cancel != nil {
			server.cancel()
		}
		server.mu.Unlock()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()
	err = httpServer.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *uiServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"application": "IMAJER", "version": version})
}

func (s *uiServer) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	home, _ := os.UserHomeDir()
	agentsDir := filepath.Join(s.resourceDir, "agents")
	agents := map[string]string{
		"linux_amd64":   filepath.Join(agentsDir, "imajer-agent-linux-amd64"),
		"linux_arm64":   filepath.Join(agentsDir, "imajer-agent-linux-arm64"),
		"windows_amd64": filepath.Join(agentsDir, "imajer-agent-windows-amd64.exe"),
	}
	localAgent := filepath.Join(s.workingDir, "dist", "imajer-agent")
	if s.packaged {
		localName := "imajer-agent-" + runtime.GOOS + "-" + runtime.GOARCH
		if runtime.GOOS == "windows" {
			localName += ".exe"
		}
		localAgent = filepath.Join(agentsDir, localName)
	}
	if runtime.GOOS == "windows" && !s.packaged {
		localAgent = filepath.Join(s.workingDir, "dist", "imajer-agent-windows-amd64.exe")
	}
	agents["local"] = localAgent
	demoJob := filepath.Join(s.workingDir, "demo", "local-job.yaml")
	demoCase := filepath.Join(s.workingDir, "demo", "evidence", "CASE-LOCAL-001", "EVID-LOCAL-001")
	demoPublicKey := filepath.Join(s.workingDir, "demo", "keys", "examiner-public.pem")
	demoAvailable := regularFileExists(demoJob) && regularFileExists(localAgent)
	defaultOutput := filepath.Join(home, "Documents", "IMAJER-Evidence")
	toolManifest := filepath.Join(agentsDir, "tool-manifest.json")
	trustPublicKey := filepath.Join(agentsDir, "tool-release-public.pem")
	if !regularFileExists(toolManifest) {
		toolManifest = ""
	}
	if !regularFileExists(trustPublicKey) {
		trustPublicKey = ""
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":               s.token,
		"working_dir":         s.workingDir,
		"home_dir":            home,
		"packaged":            s.packaged,
		"agents":              agents,
		"default_agent":       localAgent,
		"default_output":      defaultOutput,
		"default_signing_key": s.defaultSigningKey,
		"tool_manifest":       toolManifest,
		"trust_public_key":    trustPublicKey,
		"demo_available":      demoAvailable,
		"demo_job":            demoJob,
		"demo_case_dir":       demoCase,
		"demo_public_key":     demoPublicKey,
	})
}

func (s *uiServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	status := s.status
	status.Logs = append([]string(nil), s.status.Logs...)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, status)
}

func (s *uiServer) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req uiRunRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Geçersiz istek: " + err.Error()})
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action != "discover" && action != "acquire" && action != "resume" &&
		action != "verify" && action != "report" && action != "cleanup" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Desteklenmeyen işlem"})
		return
	}
	var args []string
	jobPath := strings.TrimSpace(req.JobPath)
	caseDir := strings.TrimSpace(req.CaseDir)
	if action == "verify" {
		if caseDir == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Kanıt dizini gereklidir"})
			return
		}
		args = []string{"verify", "--case-dir", absoluteFrom(s.workingDir, caseDir)}
		if strings.TrimSpace(req.PublicKey) != "" {
			args = append(args, "--public-key", absoluteFrom(s.workingDir, req.PublicKey))
		}
	} else {
		if req.Job != nil {
			var err error
			jobPath, caseDir, err = s.saveJob(*req.Job)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
		} else {
			if jobPath == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Job dosyası yolu gereklidir"})
				return
			}
			jobPath = absoluteFrom(s.workingDir, jobPath)
			if loaded, err := config.Load(jobPath); err == nil {
				caseDir = filepath.Join(loaded.Output.Directory, loaded.Case.ID, loaded.Case.EvidenceID)
			}
		}
		args = []string{action, "--job", jobPath}
	}
	if err := s.startOperation(action, args, jobPath, caseDir, req.Password); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{
		"message": "İşlem başlatıldı", "job_path": jobPath, "case_dir": caseDir,
	})
}

func (s *uiServer) handleCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	cancel := s.cancel
	running := s.status.Running
	s.mu.Unlock()
	if !running || cancel == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Çalışan işlem yok"})
		return
	}
	cancel()
	writeJSON(w, http.StatusAccepted, map[string]string{"message": "İptal isteği gönderildi; güvenli cleanup beklenecek"})
}

func (s *uiServer) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	running := s.status.Running
	s.mu.Unlock()
	if running {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Önce çalışan işlemi güvenle durdurun"})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"message": "IMAJER kapatılıyor"})
	go func() {
		time.Sleep(150 * time.Millisecond)
		select {
		case s.shutdown <- struct{}{}:
		default:
		}
	}()
}

func (s *uiServer) saveJob(form uiJobForm) (string, string, error) {
	port, err := parseOptionalInt(form.Port)
	if err != nil {
		return "", "", fmt.Errorf("port: %w", err)
	}
	diskSize, err := parseOptionalInt64(form.DiskSize)
	if err != nil {
		return "", "", fmt.Errorf("disk boyutu: %w", err)
	}
	sectorSize, err := parseOptionalInt64(form.SectorSize)
	if err != nil {
		return "", "", fmt.Errorf("sektör boyutu: %w", err)
	}
	transportName := strings.ToLower(strings.TrimSpace(form.Transport))
	if port == 0 {
		if transportName == "ssh" {
			port = 22
		} else if transportName == "winrm" {
			port = 5986
		}
	}
	job := config.Job{
		Case: config.Case{
			ID: strings.TrimSpace(form.CaseID), EvidenceID: strings.TrimSpace(form.EvidenceID),
			Examiner: strings.TrimSpace(form.Examiner), Organization: strings.TrimSpace(form.Organization),
			AuthorityRef: strings.TrimSpace(form.AuthorityRef), Notes: strings.TrimSpace(form.Notes),
			Authorized: form.Authorized,
		},
		Target: config.Target{
			Transport: transportName, Host: strings.TrimSpace(form.Host), Port: port,
			User: strings.TrimSpace(form.User), Auth: strings.ToLower(strings.TrimSpace(form.Auth)),
			PrivateKey: absOptional(s.workingDir, form.PrivateKey),
			KnownHosts: absOptional(s.workingDir, form.KnownHosts),
			CAFile:     absOptional(s.workingDir, form.CAFile),
		},
		Acquisition: config.Acquisition{
			Profile:   strings.ToLower(strings.TrimSpace(form.Profile)),
			ChunkSize: config.DefaultChunkSize, SegmentSize: config.DefaultSegmentSize,
			Disk: config.Source{
				Path: strings.TrimSpace(form.DiskPath), ID: strings.TrimSpace(form.DiskID),
				Model: strings.TrimSpace(form.DiskModel), Size: diskSize, SectorSize: sectorSize,
				Provider: strings.ToLower(strings.TrimSpace(form.DiskProvider)),
			},
			RAM: config.Source{
				ID: "volatile-memory", Provider: strings.ToLower(strings.TrimSpace(form.RAMProvider)),
				ToolName:      strings.TrimSpace(form.RAMToolName),
				ToolLocalPath: absOptional(s.workingDir, form.RAMToolLocal),
			},
		},
		Output: config.Output{
			Directory:  absoluteFrom(s.workingDir, strings.TrimSpace(form.OutputDirectory)),
			SigningKey: absOptional(s.workingDir, form.SigningKey),
		},
		Agent: config.Agent{
			LocalPath:      absOptional(s.workingDir, form.AgentLocal),
			RemotePath:     strings.TrimSpace(form.AgentRemote),
			ToolManifest:   absOptional(s.workingDir, form.ToolManifest),
			TrustPublicKey: absOptional(s.workingDir, form.TrustPublicKey),
		},
		Retry: config.Retry{
			MaxAttempts: 10, Connect: 30 * time.Second, Chunk: 5 * time.Minute,
			Cleanup: 2 * time.Minute,
		},
	}
	if job.Acquisition.Disk.Provider == "" {
		job.Acquisition.Disk.Provider = "native"
	}
	if job.Acquisition.RAM.Provider == "" {
		job.Acquisition.RAM.Provider = "auto"
	}
	if err := job.Validate(); err != nil {
		return "", "", fmt.Errorf("Form eksik veya geçersiz: %w", err)
	}
	if job.Output.SigningKey == "" {
		return "", "", errors.New("İmzalama anahtarı gereklidir")
	}
	raw, err := yaml.Marshal(job)
	if err != nil {
		return "", "", err
	}
	jobsDir := filepath.Join(job.Output.Directory, ".imajer-ui", "jobs")
	if err := os.MkdirAll(jobsDir, 0o700); err != nil {
		return "", "", fmt.Errorf("job dizini oluştur: %w", err)
	}
	jobPath := filepath.Join(jobsDir, job.Case.ID+"-"+job.Case.EvidenceID+".yaml")
	if err := evidence.AtomicWrite(jobPath, raw, 0o600); err != nil {
		return "", "", fmt.Errorf("job kaydet: %w", err)
	}
	caseDir := filepath.Join(job.Output.Directory, job.Case.ID, job.Case.EvidenceID)
	return jobPath, caseDir, nil
}

func (s *uiServer) startOperation(action string, args []string, jobPath, caseDir, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status.Running {
		return errors.New("Başka bir işlem halen çalışıyor")
	}
	opCtx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.status = uiStatus{
		Running: true, Action: action, StartedAt: time.Now().UTC(),
		Message: "İşlem çalışıyor", JobPath: jobPath, CaseDir: caseDir,
		Logs: []string{"$ imajer " + strings.Join(args, " ")},
	}
	go s.runCommand(opCtx, args, password)
	return nil
}

func (s *uiServer) runCommand(ctx context.Context, args []string, password string) {
	cmd := exec.Command(s.executable, args...)
	cmd.Dir = s.workingDir
	cmd.Env = os.Environ()
	if password != "" {
		const passwordEnv = "IMAJER_UI_EPHEMERAL_PASSWORD"
		cmd.Env = append(cmd.Env, passwordEnv+"="+password)
		if jobIndex := indexOf(args, "--job"); jobIndex >= 0 && jobIndex+1 < len(args) {
			// Persist only the environment variable name, never its value.
			if job, err := config.LoadForOverrides(args[jobIndex+1]); err == nil {
				job.Target.PasswordEnv = passwordEnv
				if raw, err := yaml.Marshal(job); err == nil {
					_ = evidence.AtomicWrite(args[jobIndex+1], raw, 0o600)
				}
			}
		}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.finishOperation(err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		s.finishOperation(err)
		return
	}
	if err := cmd.Start(); err != nil {
		s.finishOperation(err)
		return
	}
	processDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			if runtime.GOOS == "windows" {
				_ = cmd.Process.Kill()
			} else {
				// The child handles SIGINT with signal.NotifyContext and runs
				// its bounded cleanup path before exiting.
				_ = cmd.Process.Signal(os.Interrupt)
			}
		case <-processDone:
		}
	}()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		s.consumeOutput(stdout)
	}()
	go func() {
		defer wg.Done()
		s.consumeOutput(stderr)
	}()
	waitErr := cmd.Wait()
	close(processDone)
	wg.Wait()
	if ctx.Err() != nil {
		waitErr = errors.Join(waitErr, ctx.Err())
	}
	s.finishOperation(waitErr)
}

func (s *uiServer) consumeOutput(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Split(splitLinesAndCarriageReturns)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			s.appendLog(line)
		}
	}
	if err := scanner.Err(); err != nil {
		s.appendLog("Log okuma hatası: " + err.Error())
	}
}

func (s *uiServer) appendLog(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	line = redactUI(line)
	if len(s.status.Logs) >= 800 {
		copy(s.status.Logs, s.status.Logs[len(s.status.Logs)-700:])
		s.status.Logs = s.status.Logs[:700]
	}
	s.status.Logs = append(s.status.Logs, line)
}

func (s *uiServer) finishOperation(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.Running = false
	s.status.FinishedAt = time.Now().UTC()
	s.status.Success = err == nil
	if err == nil {
		s.status.Message = "İşlem başarıyla tamamlandı"
	} else if errors.Is(err, context.Canceled) {
		s.status.Message = "İşlem iptal edildi"
		s.status.Logs = append(s.status.Logs, "İptal edildi")
	} else {
		s.status.Message = "İşlem başarısız: " + redactUI(err.Error())
		s.status.Logs = append(s.status.Logs, s.status.Message)
	}
	s.cancel = nil
}

func (s *uiServer) requireToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Imajer-Token") != s.token {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func localhostOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if parsed, _, err := net.SplitHostPort(r.Host); err == nil {
			host = parsed
		}
		host = strings.Trim(host, "[]")
		if host != "127.0.0.1" && host != "localhost" && host != "::1" {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func secureToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func openBrowser(url string) error {
	var command string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		command, args = "open", []string{url}
	case "windows":
		command, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		command, args = "xdg-open", []string{url}
	}
	return exec.Command(command, args...).Start()
}

func openRunningUI(listen string) bool {
	if strings.HasSuffix(listen, ":0") {
		return false
	}
	url := "http://" + listen
	client := http.Client{Timeout: 750 * time.Millisecond}
	response, err := client.Get(url + "/api/health")
	if err != nil {
		return false
	}
	defer response.Body.Close()
	var health map[string]string
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&health) != nil ||
		health["application"] != "IMAJER" {
		return false
	}
	return openBrowser(url) == nil
}

func resolveUIRuntime(executable string) (workingDir, resourceDir string, packaged bool, err error) {
	executable, err = filepath.Abs(executable)
	if err != nil {
		return "", "", false, err
	}
	executableDir := filepath.Dir(executable)
	resourceDir = executableDir
	if runtime.GOOS == "darwin" && filepath.Base(executableDir) == "MacOS" {
		contentsDir := filepath.Dir(executableDir)
		candidate := filepath.Join(contentsDir, "Resources")
		if filepath.Base(contentsDir) == "Contents" && directoryExists(filepath.Join(candidate, "agents")) {
			resourceDir, packaged = candidate, true
		}
	} else if directoryExists(filepath.Join(executableDir, "agents")) {
		resourceDir, packaged = executableDir, true
	}
	if !packaged {
		workingDir, err = os.Getwd()
		return workingDir, workingDir, false, err
	}
	configDir, configErr := os.UserConfigDir()
	if configErr != nil {
		return "", "", false, configErr
	}
	workingDir = filepath.Join(configDir, "IMAJER")
	if err := os.MkdirAll(workingDir, 0o700); err != nil {
		return "", "", false, fmt.Errorf("uygulama veri dizini oluştur: %w", err)
	}
	return workingDir, resourceDir, true, nil
}

func ensureExaminerKey(workingDir string) (privatePath, publicPath string, err error) {
	keyDir := filepath.Join(workingDir, "keys")
	privatePath = filepath.Join(keyDir, "examiner-private.pem")
	publicPath = filepath.Join(keyDir, "examiner-public.pem")
	if regularFileExists(privatePath) && regularFileExists(publicPath) {
		return privatePath, publicPath, nil
	}
	if regularFileExists(publicPath) && !regularFileExists(privatePath) {
		return "", "", errors.New("özel imza anahtarı eksik; mevcut açık anahtarın üzerine yazılmadı")
	}
	if err := os.MkdirAll(keyDir, 0o700); err != nil {
		return "", "", err
	}
	if regularFileExists(privatePath) {
		privatePEM, readErr := os.ReadFile(privatePath)
		if readErr != nil {
			return "", "", readErr
		}
		block, _ := pem.Decode(privatePEM)
		if block == nil {
			return "", "", errors.New("özel imza anahtarı PEM biçiminde değil")
		}
		parsed, parseErr := x509.ParsePKCS8PrivateKey(block.Bytes)
		if parseErr != nil {
			return "", "", fmt.Errorf("özel imza anahtarını ayrıştır: %w", parseErr)
		}
		privateKey, ok := parsed.(ed25519.PrivateKey)
		if !ok {
			return "", "", errors.New("özel imza anahtarı Ed25519 değil")
		}
		publicDER, marshalErr := x509.MarshalPKIXPublicKey(privateKey.Public())
		if marshalErr != nil {
			return "", "", marshalErr
		}
		publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
		if err := evidence.AtomicWrite(publicPath, publicPEM, 0o644); err != nil {
			return "", "", err
		}
		return privatePath, publicPath, nil
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return "", "", err
	}
	publicDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return "", "", err
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
	if err := evidence.AtomicWrite(privatePath, privatePEM, 0o600); err != nil {
		return "", "", err
	}
	if err := evidence.AtomicWrite(publicPath, publicPEM, 0o644); err != nil {
		return "", "", err
	}
	return privatePath, publicPath, nil
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func absoluteFrom(base, path string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	result, err := filepath.Abs(filepath.Join(base, path))
	if err != nil {
		return path
	}
	return result
}

func absOptional(base, path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	return absoluteFrom(base, path)
}

func parseOptionalInt(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0, errors.New("pozitif sayı olmalıdır")
	}
	return n, nil
}

func parseOptionalInt64(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 0 {
		return 0, errors.New("pozitif sayı olmalıdır")
	}
	return n, nil
}

func splitLinesAndCarriageReturns(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for i, b := range data {
		if b == '\n' || b == '\r' {
			advance = i + 1
			for advance < len(data) && (data[advance] == '\n' || data[advance] == '\r') {
				advance++
			}
			return advance, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func redactUI(value string) string {
	value = uiCredentialAssignment.ReplaceAllString(value, "${1}=[REDACTED]")
	return uiBearerCredential.ReplaceAllString(value, "Bearer [REDACTED]")
}

var (
	uiCredentialAssignment = regexp.MustCompile(`(?i)\b(password|passphrase|secret|token|api[_-]?key|authorization|credential)\s*[:=]\s*[^ \t\r\n&,;]+`)
	uiBearerCredential     = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]+`)
)

func indexOf(values []string, wanted string) int {
	for i, value := range values {
		if value == wanted {
			return i
		}
	}
	return -1
}
