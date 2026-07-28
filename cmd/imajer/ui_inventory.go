package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hakankrgz/imajer/internal/config"
	"github.com/hakankrgz/imajer/internal/transport"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const inventoryOutputLimit = 4 << 20

type uiInventory struct {
	Hostname   string   `json:"hostname"`
	OS         string   `json:"os"`
	Arch       string   `json:"arch"`
	Admin      bool     `json:"admin"`
	Privilege  string   `json:"privilege,omitempty"`
	Disks      []uiDisk `json:"disks"`
	AgentLocal string   `json:"agent_local"`
	Warnings   []string `json:"warnings,omitempty"`
}

type uiDisk struct {
	Path        string   `json:"path"`
	ID          string   `json:"id"`
	Serial      string   `json:"serial,omitempty"`
	Model       string   `json:"model,omitempty"`
	Size        int64    `json:"size"`
	SectorSize  int64    `json:"sector_size"`
	StableID    bool     `json:"stable_id"`
	Mounted     bool     `json:"mounted"`
	Mountpoints []string `json:"mountpoints,omitempty"`
}

type commandResult struct {
	data []byte
	err  error
}

type uiSSHHostKey struct {
	Host        string        `json:"host"`
	Algorithm   string        `json:"algorithm"`
	Fingerprint string        `json:"fingerprint"`
	Trusted     bool          `json:"trusted"`
	Changed     bool          `json:"changed"`
	key         ssh.PublicKey `json:"-"`
}

func ensureManagedKnownHosts(workingDir string, packaged bool) (string, error) {
	dir := filepath.Join(workingDir, "ssh")
	if !packaged {
		dir = filepath.Join(workingDir, ".imajer-ui", "ssh")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "known_hosts")
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return "", errors.New("known_hosts güven deposu normal bir dosya değil")
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func (s *uiServer) handleSSHHostKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.inventoryMu.TryLock() {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Başka bir hedef kontrolü zaten çalışıyor"})
		return
	}
	defer s.inventoryMu.Unlock()

	s.mu.Lock()
	running := s.status.Running
	s.mu.Unlock()
	if running {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Çalışan işlem varken SSH anahtarı denetlenemez"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var request uiRunRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Geçersiz istek: " + err.Error()})
		return
	}
	if request.Job == nil || !strings.EqualFold(strings.TrimSpace(request.Job.Transport), "ssh") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "SSH hedef bilgileri eksik"})
		return
	}
	host := strings.TrimSpace(request.Job.Host)
	if host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Sunucu adı veya IP adresi gereklidir"})
		return
	}
	port, err := parseOptionalInt(request.Job.Port)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Port: " + err.Error()})
		return
	}
	if port == 0 {
		port = 22
	}
	knownHostsPath := absOptional(s.workingDir, request.Job.KnownHosts)
	if knownHostsPath == "" {
		knownHostsPath = s.defaultKnownHosts
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	status, err := probeSSHHostKey(ctx, host, port, knownHostsPath)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": redactUI("SSH sunucu anahtarı okunamadı: " + err.Error()),
		})
		return
	}
	if status.Changed {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "SSH sunucu anahtarı daha önce güvenilen anahtarla eşleşmiyor. Olası saldırı veya anahtar değişimi; bağlantı durduruldu.",
		})
		return
	}
	if request.TrustHostKey && !status.Trusted {
		if !sameLocalPath(knownHostsPath, s.defaultKnownHosts) {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "Özel known_hosts dosyası uygulama tarafından değiştirilemez; anahtarı dosyaya kurum prosedürünüzle ekleyin",
			})
			return
		}
		if strings.TrimSpace(request.ExpectedFingerprint) == "" ||
			request.ExpectedFingerprint != status.Fingerprint {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "SSH fingerprint denetim sırasında değişti; güven kaydı yapılmadı",
			})
			return
		}
		if err := appendTrustedHostKey(knownHostsPath, status.Host, status.key); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "SSH güven kaydı yazılamadı: " + err.Error(),
			})
			return
		}
		status.Trusted = true
	}
	writeJSON(w, http.StatusOK, status)
}

func probeSSHHostKey(ctx context.Context, host string, port int, knownHostsPath string) (uiSSHHostKey, error) {
	if knownHostsPath == "" {
		return uiSSHHostKey{}, errors.New("known_hosts güven deposu belirtilmedi")
	}
	address := net.JoinHostPort(host, strconv.Itoa(port))
	dialer := net.Dialer{Timeout: 10 * time.Second}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return uiSSHHostKey{}, err
	}
	defer connection.Close()
	return inspectSSHHostKey(ctx, connection, address, knownHostsPath)
}

func inspectSSHHostKey(
	ctx context.Context,
	connection net.Conn,
	address string,
	knownHostsPath string,
) (uiSSHHostKey, error) {
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	}

	errCaptured := errors.New("SSH host key captured")
	var captured ssh.PublicKey
	clientConfig := &ssh.ClientConfig{
		User:    "imajer-host-key-probe",
		Timeout: 10 * time.Second,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			captured = key
			return errCaptured
		},
	}
	_, _, _, handshakeErr := ssh.NewClientConn(connection, address, clientConfig)
	if captured == nil {
		if handshakeErr == nil {
			handshakeErr = errors.New("sunucu bir SSH host key sunmadı")
		}
		return uiSSHHostKey{}, handshakeErr
	}

	checker, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return uiSSHHostKey{}, fmt.Errorf("known_hosts: %w", err)
	}
	checkErr := checker(address, connection.RemoteAddr(), captured)
	status := uiSSHHostKey{
		Host:        knownhosts.Normalize(address),
		Algorithm:   captured.Type(),
		Fingerprint: ssh.FingerprintSHA256(captured),
		key:         captured,
	}
	if checkErr == nil {
		status.Trusted = true
		return status, nil
	}
	var keyError *knownhosts.KeyError
	if !errors.As(checkErr, &keyError) {
		return uiSSHHostKey{}, fmt.Errorf("known_hosts doğrulaması: %w", checkErr)
	}
	status.Changed = len(keyError.Want) > 0
	return status, nil
}

func appendTrustedHostKey(path, host string, key ssh.PublicKey) error {
	if key == nil {
		return errors.New("boş SSH host key kaydedilemez")
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	line := knownhosts.Line([]string{host}, key) + "\n"
	if _, err := io.WriteString(file, line); err != nil {
		return err
	}
	return file.Sync()
}

func sameLocalPath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func (s *uiServer) handleInventory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.inventoryMu.TryLock() {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Hedef taraması zaten çalışıyor"})
		return
	}
	defer s.inventoryMu.Unlock()

	s.mu.Lock()
	running := s.status.Running
	s.mu.Unlock()
	if running {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Çalışan işlem varken hedef taranamaz"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var request uiRunRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Geçersiz istek: " + err.Error()})
		return
	}
	if request.Job == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Hedef bağlantı bilgileri eksik"})
		return
	}
	target, err := s.targetFromForm(*request.Job, request.Password)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	connection, err := transport.New(ctx, target, 20*time.Second)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": redactUI("Hedefe bağlanılamadı: " + err.Error()),
		})
		return
	}
	defer connection.Close()

	inventory, err := collectRemoteInventory(ctx, connection)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": redactUI("Disk envanteri alınamadı: " + err.Error()),
		})
		return
	}
	agentPath, err := s.agentPathFor(inventory.OS, inventory.Arch)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	inventory.AgentLocal = agentPath
	if !inventory.Admin {
		inventory.Warnings = append(inventory.Warnings, "Root veya parolasız sudo doğrulanamadı; edinim başlamadan önce 'sudo -n true' çalışmalıdır")
	}
	if len(inventory.Disks) == 0 {
		inventory.Warnings = append(inventory.Warnings, "Hedefte seçilebilir fiziksel disk bulunamadı")
	}
	if inventory.OS == "linux" && inventory.Arch == "arm64" {
		inventory.Warnings = append(inventory.Warnings, "Raspberry Pi/ARM64 disk edinimi desteklenir; RAM edinimi için hedef çekirdekle birebir uyumlu, imzalı LiME modülü gerekir")
	}
	for _, disk := range inventory.Disks {
		if !disk.StableID {
			inventory.Warnings = append(inventory.Warnings, disk.Path+" seri/WWN bildirmiyor; kimlik yol, model, boyut ve sektör bilgisiyle doğrulanacak")
		}
		if disk.Mounted {
			inventory.Warnings = append(inventory.Warnings, disk.Path+" bağlı dosya sistemleri içeriyor; canlı disk edinimi tek bir atomik zamanı temsil etmez")
		}
	}
	writeJSON(w, http.StatusOK, inventory)
}

func (s *uiServer) targetFromForm(form uiJobForm, password string) (config.Target, error) {
	transportName := strings.ToLower(strings.TrimSpace(form.Transport))
	if transportName != "ssh" && transportName != "winrm" {
		return config.Target{}, errors.New("Disk keşfi için Linux / SSH veya Windows / WinRM seçin")
	}
	port, err := parseOptionalInt(form.Port)
	if err != nil {
		return config.Target{}, fmt.Errorf("Port: %w", err)
	}
	if port == 0 {
		if transportName == "ssh" {
			port = 22
		} else {
			port = 5986
		}
	}
	target := config.Target{
		Transport: transportName,
		Host:      strings.TrimSpace(form.Host),
		Port:      port,
		User:      strings.TrimSpace(form.User),
		Auth:      strings.ToLower(strings.TrimSpace(form.Auth)),
		PrivateKey: absOptional(
			s.workingDir,
			form.PrivateKey,
		),
		KnownHosts: absOptional(s.workingDir, form.KnownHosts),
		CAFile:     absOptional(s.workingDir, form.CAFile),
	}
	target.RuntimePassword = password
	if target.Host == "" {
		return config.Target{}, errors.New("Sunucu adı veya IP adresi gereklidir")
	}
	if target.User == "" {
		return config.Target{}, errors.New("Kullanıcı adı gereklidir")
	}
	if transportName == "ssh" && target.KnownHosts == "" {
		return config.Target{}, errors.New("SSH sunucu kimliğini doğrulamak için known_hosts dosyası gereklidir")
	}
	if transportName == "winrm" && target.Auth == "" {
		target.Auth = "ntlm"
	}
	return target, nil
}

func (s *uiServer) agentPathFor(osName, arch string) (string, error) {
	osName = strings.ToLower(strings.TrimSpace(osName))
	arch = normalizeArchitecture(arch)
	if osName != "linux" && osName != "windows" {
		return "", fmt.Errorf("Hedef işletim sistemi desteklenmiyor: %s", osName)
	}
	if arch != "amd64" && arch != "arm64" {
		return "", fmt.Errorf("Hedef mimarisi desteklenmiyor: %s", arch)
	}
	if osName == "windows" && arch != "amd64" {
		return "", errors.New("Uzak Windows edinimi şu anda yalnız x64 (amd64) hedefleri destekliyor")
	}
	name := "imajer-agent-" + osName + "-" + arch
	if osName == "windows" {
		name += ".exe"
	}
	path := filepath.Join(s.agentDirectory(), name)
	if !regularFileExists(path) {
		return "", fmt.Errorf("Paket içinde %s/%s agent bulunamadı", osName, arch)
	}
	return path, nil
}

func collectRemoteInventory(ctx context.Context, connection transport.Transport) (uiInventory, error) {
	switch connection.Name() {
	case "ssh":
		return collectSSHInventory(ctx, connection)
	case "winrm":
		return collectWinRMInventory(ctx, connection)
	default:
		return uiInventory{}, errors.New("Uzak disk keşfi yalnız SSH ve WinRM için kullanılabilir")
	}
}

func collectSSHInventory(ctx context.Context, connection transport.Transport) (uiInventory, error) {
	archRaw, _, err := runTransportCommand(ctx, connection, []string{"uname", "-m"})
	if err != nil {
		return uiInventory{}, fmt.Errorf("mimari sorgusu: %w", err)
	}
	hostRaw, _, err := runTransportCommand(ctx, connection, []string{"hostname"})
	if err != nil {
		return uiInventory{}, fmt.Errorf("hostname sorgusu: %w", err)
	}
	uidRaw, _, uidErr := runTransportCommand(ctx, connection, []string{"id", "-u"})
	admin := uidErr == nil && strings.TrimSpace(string(uidRaw)) == "0"
	privilege := "unprivileged"
	if admin {
		privilege = "root"
	} else {
		sudoRaw, _, sudoErr := runTransportCommand(ctx, connection, []string{"sudo", "-n", "id", "-u"})
		if sudoErr == nil && strings.TrimSpace(string(sudoRaw)) == "0" {
			admin = true
			privilege = "passwordless_sudo"
		}
	}
	storageRaw, stderr, err := runTransportCommand(ctx, connection, []string{
		"lsblk", "-b", "-J", "-o", "NAME,PATH,MODEL,SERIAL,WWN,SIZE,LOG-SEC,TYPE,MOUNTPOINT",
	})
	if err != nil {
		return uiInventory{}, fmt.Errorf("lsblk: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	disks, err := parseLSBLKDisks(storageRaw)
	if err != nil {
		return uiInventory{}, err
	}
	return uiInventory{
		Hostname:  strings.TrimSpace(string(hostRaw)),
		OS:        "linux",
		Arch:      normalizeArchitecture(string(archRaw)),
		Admin:     admin,
		Privilege: privilege,
		Disks:     disks,
	}, nil
}

func collectWinRMInventory(ctx context.Context, connection transport.Transport) (uiInventory, error) {
	script := `$p=[Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent();` +
		`$admin=$p.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator);` +
		`$disks=@(Get-CimInstance Win32_DiskDrive | ForEach-Object {` +
		`[pscustomobject]@{Path=$_.DeviceID;Serial=[string]$_.SerialNumber;Model=[string]$_.Model;` +
		`Size=[int64]$_.Size;SectorSize=[int64]$_.BytesPerSector}});` +
		`[pscustomobject]@{Hostname=$env:COMPUTERNAME;Arch=$env:PROCESSOR_ARCHITECTURE;Admin=$admin;Disks=$disks}` +
		`|ConvertTo-Json -Depth 4 -Compress`
	stdout, stderr, err := runTransportCommand(ctx, connection, []string{
		"powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script,
	})
	if err != nil {
		return uiInventory{}, fmt.Errorf("PowerShell/CIM: %w: %s", err, strings.TrimSpace(string(stderr)))
	}
	return parseWindowsInventory(stdout)
}

func runTransportCommand(ctx context.Context, connection transport.Transport, argv []string) ([]byte, []byte, error) {
	session, err := connection.Start(ctx, argv)
	if err != nil {
		return nil, nil, err
	}
	defer session.Close()
	stdoutChannel := make(chan commandResult, 1)
	stderrChannel := make(chan commandResult, 1)
	go readLimited(session.Stdout, stdoutChannel)
	go readLimited(session.Stderr, stderrChannel)
	waitErr := session.Wait()
	stdout := <-stdoutChannel
	stderr := <-stderrChannel
	return stdout.data, stderr.data, errors.Join(waitErr, stdout.err, stderr.err)
}

func readLimited(reader io.Reader, result chan<- commandResult) {
	raw, err := io.ReadAll(io.LimitReader(reader, inventoryOutputLimit+1))
	if err == nil && len(raw) > inventoryOutputLimit {
		err = errors.New("uzak komut çıktısı güvenli sınırı aştı")
	}
	result <- commandResult{data: raw, err: err}
}

func parseLSBLKDisks(raw []byte) ([]uiDisk, error) {
	type lsblkRow struct {
		Name       json.RawMessage `json:"name"`
		Path       json.RawMessage `json:"path"`
		Model      json.RawMessage `json:"model"`
		Serial     json.RawMessage `json:"serial"`
		WWN        json.RawMessage `json:"wwn"`
		Size       json.RawMessage `json:"size"`
		SectorSize json.RawMessage `json:"log-sec"`
		Type       json.RawMessage `json:"type"`
		Mountpoint json.RawMessage `json:"mountpoint"`
		Children   []lsblkRow      `json:"children"`
	}
	var envelope struct {
		BlockDevices []lsblkRow `json:"blockdevices"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("lsblk JSON ayrıştırma: %w", err)
	}
	var disks []uiDisk
	for _, row := range envelope.BlockDevices {
		if !strings.EqualFold(rawText(row.Type), "disk") {
			continue
		}
		name := rawText(row.Name)
		path := rawText(row.Path)
		if path == "" && name != "" {
			path = "/dev/" + name
		}
		serial := rawText(row.Serial)
		wwn := rawText(row.WWN)
		stableID := serial
		if stableID == "" {
			stableID = wwn
		}
		if stableID == "" {
			stableID = path
		}
		size := rawInt64(row.Size)
		sector := rawInt64(row.SectorSize)
		if sector <= 0 {
			sector = 512
		}
		if path == "" || size <= 0 {
			continue
		}
		var mountpoints []string
		var collectMountpoints func(lsblkRow)
		collectMountpoints = func(current lsblkRow) {
			if mountpoint := strings.TrimSpace(rawText(current.Mountpoint)); mountpoint != "" {
				mountpoints = append(mountpoints, mountpoint)
			}
			for _, child := range current.Children {
				collectMountpoints(child)
			}
		}
		collectMountpoints(row)
		sort.Strings(mountpoints)
		disks = append(disks, uiDisk{
			Path: path, ID: stableID, Serial: serial, Model: rawText(row.Model),
			Size: size, SectorSize: sector, Mounted: len(mountpoints) > 0,
			Mountpoints: mountpoints, StableID: serial != "" || wwn != "",
		})
	}
	return disks, nil
}

func parseWindowsInventory(raw []byte) (uiInventory, error) {
	var envelope struct {
		Hostname string          `json:"Hostname"`
		Arch     string          `json:"Arch"`
		Admin    bool            `json:"Admin"`
		Disks    json.RawMessage `json:"Disks"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return uiInventory{}, fmt.Errorf("Windows envanter JSON ayrıştırma: %w", err)
	}
	type diskRow struct {
		Path       string `json:"Path"`
		Serial     string `json:"Serial"`
		Model      string `json:"Model"`
		Size       int64  `json:"Size"`
		SectorSize int64  `json:"SectorSize"`
	}
	var rows []diskRow
	if len(envelope.Disks) > 0 && string(envelope.Disks) != "null" {
		if err := json.Unmarshal(envelope.Disks, &rows); err != nil {
			var single diskRow
			if singleErr := json.Unmarshal(envelope.Disks, &single); singleErr != nil {
				return uiInventory{}, fmt.Errorf("Windows disk listesi ayrıştırma: %w", err)
			}
			rows = []diskRow{single}
		}
	}
	inventory := uiInventory{
		Hostname: envelope.Hostname, OS: "windows",
		Arch: normalizeArchitecture(envelope.Arch), Admin: envelope.Admin,
	}
	for _, row := range rows {
		if row.Path == "" || row.Size <= 0 {
			continue
		}
		id := strings.TrimSpace(row.Serial)
		if id == "" {
			id = row.Path
		}
		sector := row.SectorSize
		if sector <= 0 {
			sector = 512
		}
		inventory.Disks = append(inventory.Disks, uiDisk{
			Path: row.Path, ID: id, Serial: strings.TrimSpace(row.Serial),
			Model: strings.TrimSpace(row.Model), Size: row.Size, SectorSize: sector,
			StableID: strings.TrimSpace(row.Serial) != "",
		})
	}
	return inventory, nil
}

func normalizeArchitecture(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "x86_64", "amd64", "x64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func rawText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return strings.TrimSpace(text)
	}
	return strings.Trim(strings.TrimSpace(string(raw)), `"`)
}

func rawInt64(raw json.RawMessage) int64 {
	value := rawText(raw)
	result, _ := strconv.ParseInt(value, 10, 64)
	return result
}
