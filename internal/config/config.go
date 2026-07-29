package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	DefaultChunkSize   int64 = 8 << 20
	DefaultSegmentSize int64 = 2 << 30
)

type Job struct {
	Case        Case        `yaml:"case" json:"case"`
	Target      Target      `yaml:"target" json:"target"`
	Acquisition Acquisition `yaml:"acquisition" json:"acquisition"`
	Output      Output      `yaml:"output" json:"output"`
	Agent       Agent       `yaml:"agent" json:"agent"`
	Retry       Retry       `yaml:"retry" json:"retry"`
}

type Case struct {
	ID           string `yaml:"id" json:"id"`
	EvidenceID   string `yaml:"evidence_id" json:"evidence_id"`
	Examiner     string `yaml:"examiner" json:"examiner"`
	Organization string `yaml:"organization" json:"organization"`
	AuthorityRef string `yaml:"authority_ref" json:"authority_ref"`
	Notes        string `yaml:"notes,omitempty" json:"notes,omitempty"`
	Authorized   bool   `yaml:"authorized" json:"authorized"`
}

type Target struct {
	Transport       string `yaml:"transport" json:"transport"`
	Host            string `yaml:"host" json:"host"`
	Port            int    `yaml:"port,omitempty" json:"port,omitempty"`
	User            string `yaml:"user" json:"user"`
	PasswordEnv     string `yaml:"password_env,omitempty" json:"password_env,omitempty"`
	PrivateKey      string `yaml:"private_key,omitempty" json:"private_key,omitempty"`
	KnownHosts      string `yaml:"known_hosts,omitempty" json:"known_hosts,omitempty"`
	CAFile          string `yaml:"ca_file,omitempty" json:"ca_file,omitempty"`
	CertFingerprint string `yaml:"cert_fingerprint,omitempty" json:"cert_fingerprint,omitempty"`
	Auth            string `yaml:"auth,omitempty" json:"auth,omitempty"`
	KerberosRealm   string `yaml:"kerberos_realm,omitempty" json:"kerberos_realm,omitempty"`
	KerberosConfig  string `yaml:"kerberos_config,omitempty" json:"kerberos_config,omitempty"`
	KerberosSPN     string `yaml:"kerberos_spn,omitempty" json:"kerberos_spn,omitempty"`
	KerberosCCache  string `yaml:"kerberos_ccache,omitempty" json:"kerberos_ccache,omitempty"`
	Insecure        bool   `yaml:"insecure,omitempty" json:"insecure,omitempty"`
	RuntimePassword string `yaml:"-" json:"-"`
}

type Acquisition struct {
	Profile     string `yaml:"profile" json:"profile"`
	Disk        Source `yaml:"disk,omitempty" json:"disk,omitempty"`
	RAM         Source `yaml:"ram,omitempty" json:"ram,omitempty"`
	ChunkSize   int64  `yaml:"chunk_size,omitempty" json:"chunk_size,omitempty"`
	SegmentSize int64  `yaml:"segment_size,omitempty" json:"segment_size,omitempty"`
}

type Source struct {
	Path           string `yaml:"path,omitempty" json:"path,omitempty"`
	ID             string `yaml:"id,omitempty" json:"id,omitempty"`
	Model          string `yaml:"model,omitempty" json:"model,omitempty"`
	Size           int64  `yaml:"size,omitempty" json:"size,omitempty"`
	SectorSize     int64  `yaml:"sector_size,omitempty" json:"sector_size,omitempty"`
	Provider       string `yaml:"provider,omitempty" json:"provider,omitempty"`
	ToolPath       string `yaml:"tool_path,omitempty" json:"tool_path,omitempty"`
	ToolName       string `yaml:"tool_name,omitempty" json:"tool_name,omitempty"`
	ToolLocalPath  string `yaml:"tool_local_path,omitempty" json:"tool_local_path,omitempty"`
	ToolRemotePath string `yaml:"tool_remote_path,omitempty" json:"tool_remote_path,omitempty"`
}

type Output struct {
	Directory  string `yaml:"directory" json:"directory"`
	SigningKey string `yaml:"signing_key,omitempty" json:"signing_key,omitempty"`
}

type Agent struct {
	LocalPath      string `yaml:"local_path,omitempty" json:"local_path,omitempty"`
	RemotePath     string `yaml:"remote_path,omitempty" json:"remote_path,omitempty"`
	ToolManifest   string `yaml:"tool_manifest,omitempty" json:"tool_manifest,omitempty"`
	TrustPublicKey string `yaml:"trust_public_key,omitempty" json:"trust_public_key,omitempty"`
	KeepOnFailure  bool   `yaml:"keep_on_failure,omitempty" json:"keep_on_failure,omitempty"`
}

type Retry struct {
	MaxAttempts int           `yaml:"max_attempts,omitempty" json:"max_attempts,omitempty"`
	Connect     time.Duration `yaml:"connect_timeout,omitempty" json:"connect_timeout,omitempty"`
	Chunk       time.Duration `yaml:"chunk_timeout,omitempty" json:"chunk_timeout,omitempty"`
	Cleanup     time.Duration `yaml:"cleanup_timeout,omitempty" json:"cleanup_timeout,omitempty"`
}

func (r *Retry) UnmarshalYAML(node *yaml.Node) error {
	var raw struct {
		MaxAttempts int    `yaml:"max_attempts"`
		Connect     string `yaml:"connect_timeout"`
		Chunk       string `yaml:"chunk_timeout"`
		Cleanup     string `yaml:"cleanup_timeout"`
	}
	if err := node.Decode(&raw); err != nil {
		return err
	}
	r.MaxAttempts = raw.MaxAttempts
	var err error
	if raw.Connect != "" {
		r.Connect, err = time.ParseDuration(raw.Connect)
		if err != nil {
			return fmt.Errorf("invalid connect_timeout: %w", err)
		}
	}
	if raw.Chunk != "" {
		r.Chunk, err = time.ParseDuration(raw.Chunk)
		if err != nil {
			return fmt.Errorf("invalid chunk_timeout: %w", err)
		}
	}
	if raw.Cleanup != "" {
		r.Cleanup, err = time.ParseDuration(raw.Cleanup)
		if err != nil {
			return fmt.Errorf("invalid cleanup_timeout: %w", err)
		}
	}
	return nil
}

func (r Retry) MarshalYAML() (any, error) {
	return struct {
		MaxAttempts int    `yaml:"max_attempts,omitempty"`
		Connect     string `yaml:"connect_timeout,omitempty"`
		Chunk       string `yaml:"chunk_timeout,omitempty"`
		Cleanup     string `yaml:"cleanup_timeout,omitempty"`
	}{
		MaxAttempts: r.MaxAttempts,
		Connect:     durationString(r.Connect),
		Chunk:       durationString(r.Chunk),
		Cleanup:     durationString(r.Cleanup),
	}, nil
}

func durationString(value time.Duration) string {
	if value == 0 {
		return ""
	}
	return value.String()
}

func Load(path string) (*Job, error) {
	j, err := LoadForOverrides(path)
	if err != nil {
		return nil, err
	}
	if err := j.Validate(); err != nil {
		return nil, err
	}
	return j, nil
}

// LoadForOverrides decodes a job and applies safe defaults without validating
// required acquisition fields. CLI callers use it to apply flag overrides
// before the final validation pass.
func LoadForOverrides(path string) (*Job, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open job: %w", err)
	}
	defer f.Close()
	var j Job
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(&j); err != nil {
		return nil, fmt.Errorf("decode job: %w", err)
	}
	j.setDefaults()
	return &j, nil
}

func (j *Job) setDefaults() {
	if j.Acquisition.ChunkSize == 0 {
		j.Acquisition.ChunkSize = DefaultChunkSize
	}
	if j.Acquisition.SegmentSize == 0 {
		j.Acquisition.SegmentSize = DefaultSegmentSize
	}
	if j.Retry.MaxAttempts == 0 {
		j.Retry.MaxAttempts = 10
	}
	if j.Retry.Connect == 0 {
		j.Retry.Connect = 30 * time.Second
	}
	if j.Retry.Chunk == 0 {
		j.Retry.Chunk = 5 * time.Minute
	}
	if j.Retry.Cleanup == 0 {
		j.Retry.Cleanup = 2 * time.Minute
	}
	if j.Target.Port == 0 {
		switch strings.ToLower(j.Target.Transport) {
		case "ssh":
			j.Target.Port = 22
		case "winrm":
			j.Target.Port = 5986
		}
	}
}

func (j *Job) Validate() error {
	var errs []error
	if !j.Case.Authorized {
		errs = append(errs, errors.New("case.authorized must be true"))
	}
	for name, value := range map[string]string{
		"case.id": j.Case.ID, "case.evidence_id": j.Case.EvidenceID,
		"case.examiner": j.Case.Examiner, "case.authority_ref": j.Case.AuthorityRef,
		"output.directory": j.Output.Directory, "target.transport": j.Target.Transport,
	} {
		if strings.TrimSpace(value) == "" {
			errs = append(errs, fmt.Errorf("%s is required", name))
		}
	}
	if !safeID(j.Case.ID) || !safeID(j.Case.EvidenceID) {
		errs = append(errs, errors.New("case and evidence IDs may contain only letters, digits, '.', '_' and '-'"))
	}
	switch strings.ToLower(j.Target.Transport) {
	case "local":
	case "ssh":
		if j.Target.Host == "" || j.Target.User == "" || j.Target.KnownHosts == "" {
			errs = append(errs, errors.New("SSH requires host, user and known_hosts"))
		}
	case "winrm":
		if j.Target.Host == "" || j.Target.User == "" {
			errs = append(errs, errors.New("WinRM requires host and user"))
		}
		if j.Target.Insecure {
			errs = append(errs, errors.New("unencrypted/insecure WinRM is forbidden"))
		}
		if strings.EqualFold(j.Target.Auth, "kerberos") &&
			(j.Target.KerberosConfig == "" || j.Target.KerberosSPN == "" ||
				(j.Target.KerberosCCache == "" && j.Target.KerberosRealm == "")) {
			errs = append(errs, errors.New("Kerberos requires kerberos_config, kerberos_spn and either kerberos_ccache or kerberos_realm"))
		}
	default:
		errs = append(errs, fmt.Errorf("unsupported transport %q", j.Target.Transport))
	}
	switch strings.ToLower(j.Acquisition.Profile) {
	case "disk":
		if j.Acquisition.Disk.Path == "" || j.Acquisition.Disk.ID == "" || j.Acquisition.Disk.Size <= 0 {
			errs = append(errs, errors.New("disk profile requires path, stable id and positive size"))
		}
	case "ram":
	case "both":
		if j.Acquisition.Disk.Path == "" || j.Acquisition.Disk.ID == "" || j.Acquisition.Disk.Size <= 0 {
			errs = append(errs, errors.New("both profile requires disk path, stable id and positive size"))
		}
	default:
		errs = append(errs, errors.New("profile must be disk, ram or both"))
	}
	if j.Acquisition.ChunkSize < 1<<20 || j.Acquisition.ChunkSize > 64<<20 {
		errs = append(errs, errors.New("chunk_size must be between 1 MiB and 64 MiB"))
	}
	if j.Acquisition.SegmentSize < j.Acquisition.ChunkSize || j.Acquisition.SegmentSize > 4<<30 {
		errs = append(errs, errors.New("segment_size must be >= chunk_size and <= 4 GiB"))
	}
	if j.Acquisition.SegmentSize%j.Acquisition.ChunkSize != 0 {
		errs = append(errs, errors.New("segment_size must be an exact multiple of chunk_size"))
	}
	if j.Retry.MaxAttempts < 1 || j.Retry.MaxAttempts > 100 {
		errs = append(errs, errors.New("max_attempts must be between 1 and 100"))
	}
	if j.Retry.Connect < time.Second || j.Retry.Connect > 10*time.Minute {
		errs = append(errs, errors.New("connect_timeout must be between 1s and 10m"))
	}
	if j.Retry.Chunk < time.Second || j.Retry.Chunk > 24*time.Hour {
		errs = append(errs, errors.New("chunk_timeout must be between 1s and 24h"))
	}
	if j.Retry.Cleanup < time.Second || j.Retry.Cleanup > 30*time.Minute {
		errs = append(errs, errors.New("cleanup_timeout must be between 1s and 30m"))
	}
	if j.Output.SigningKey != "" && !filepath.IsAbs(j.Output.SigningKey) {
		errs = append(errs, errors.New("signing_key must be an absolute path"))
	}
	if j.Output.SigningKey != "" && j.Output.Directory != "" {
		outAbs, outErr := filepath.Abs(j.Output.Directory)
		keyAbs, keyErr := filepath.Abs(j.Output.SigningKey)
		if outErr == nil && keyErr == nil {
			rel, relErr := filepath.Rel(outAbs, keyAbs)
			if relErr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				errs = append(errs, errors.New("signing_key must be stored outside the evidence output tree"))
			}
		}
	}
	return errors.Join(errs...)
}

func safeID(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') &&
			!(r >= '0' && r <= '9') && r != '.' && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

func Password(t Target) string {
	if t.RuntimePassword != "" {
		return t.RuntimePassword
	}
	if t.PasswordEnv == "" {
		return ""
	}
	return os.Getenv(t.PasswordEnv)
}
