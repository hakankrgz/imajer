package evidence

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Status string

const (
	StatusRunning                Status = "running"
	StatusVerifiedContinuous     Status = "verified_continuous"
	StatusChunkVerifiedComposite Status = "chunk_verified_composite"
	StatusIncomplete             Status = "incomplete"
	StatusFailed                 Status = "failed"
)

type State struct {
	Version          int       `json:"version"`
	CaseID           string    `json:"case_id"`
	EvidenceID       string    `json:"evidence_id"`
	ArtifactID       string    `json:"artifact_id"`
	Kind             string    `json:"kind"`
	SourceID         string    `json:"source_id"`
	SourcePath       string    `json:"source_path"`
	SourceModel      string    `json:"source_model,omitempty"`
	SourceSize       int64     `json:"source_size"`
	ExpectedSize     int64     `json:"expected_size,omitempty"`
	SectorSize       int64     `json:"sector_size,omitempty"`
	ChunkSize        int64     `json:"chunk_size"`
	SegmentSize      int64     `json:"segment_size"`
	NextOffset       int64     `json:"next_offset"`
	StartedAt        time.Time `json:"started_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	CompletedAt      time.Time `json:"completed_at,omitempty"`
	Status           Status    `json:"status"`
	Verification     string    `json:"verification,omitempty"`
	LogicalSHA256    string    `json:"logical_sha256,omitempty"`
	MerkleRoot       string    `json:"merkle_root,omitempty"`
	RemoteStreamHash string    `json:"remote_stream_sha256,omitempty"`
	Resumed          bool      `json:"resumed"`
	SessionCount     int       `json:"session_count"`
	RetryCount       int       `json:"retry_count"`
	Providers        []string  `json:"providers,omitempty"`
	LastError        string    `json:"last_error,omitempty"`
}

type ChunkRecord struct {
	SessionID string    `json:"session_id"`
	Sequence  uint64    `json:"sequence"`
	Offset    int64     `json:"offset"`
	Length    int       `json:"length"`
	SHA256    string    `json:"sha256"`
	ReadAt    time.Time `json:"read_at"`
	WrittenAt time.Time `json:"written_at"`
}

type SessionRecord struct {
	ID           string    `json:"id"`
	StartedAt    time.Time `json:"started_at"`
	EndedAt      time.Time `json:"ended_at"`
	StartOffset  int64     `json:"start_offset"`
	EndOffset    int64     `json:"end_offset"`
	RemoteSHA256 string    `json:"remote_sha256"`
	LocalSHA256  string    `json:"local_sha256"`
	Bytes        int64     `json:"bytes"`
	Provider     string    `json:"provider"`
	Error        string    `json:"error,omitempty"`
}

func LoadState(dir string) (*State, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("decode state: %w", err)
	}
	if s.Version != 1 {
		return nil, fmt.Errorf("unsupported state version %d", s.Version)
	}
	return &s, nil
}

func SaveState(dir string, s *State) error {
	s.Version = 1
	s.UpdatedAt = time.Now().UTC()
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return AtomicWrite(filepath.Join(dir, "state.json"), raw, 0o600)
}

func AtomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		tmp.Close()
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := replaceFile(tmpName, path); err != nil {
		return err
	}
	ok = true
	return syncDir(dir)
}

type ChunkLog struct {
	f *os.File
	w *bufio.Writer
}

func OpenChunkLog(dir string) (*ChunkLog, error) {
	f, err := os.OpenFile(filepath.Join(dir, "chunks.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &ChunkLog{f: f, w: bufio.NewWriterSize(f, 64<<10)}, nil
}

func (l *ChunkLog) Append(rec ChunkRecord) error {
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if _, err := l.w.Write(append(raw, '\n')); err != nil {
		return err
	}
	if err := l.w.Flush(); err != nil {
		return err
	}
	return l.f.Sync()
}

func (l *ChunkLog) Close() error {
	if l == nil || l.f == nil {
		return nil
	}
	if err := l.w.Flush(); err != nil {
		l.f.Close()
		return err
	}
	return l.f.Close()
}

func ReadChunks(dir string) ([]ChunkRecord, error) {
	f, err := os.Open(filepath.Join(dir, "chunks.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var records []ChunkRecord
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64<<10), 1<<20)
	for s.Scan() {
		var r ChunkRecord
		if err := json.Unmarshal(s.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("decode chunk log: %w", err)
		}
		records = append(records, r)
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Offset < records[j].Offset })
	return records, nil
}

func AppendSession(dir string, rec SessionRecord) error {
	path := filepath.Join(dir, "sessions.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

func ReadSessions(dir string) ([]SessionRecord, error) {
	f, err := os.Open(filepath.Join(dir, "sessions.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var records []SessionRecord
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64<<10), 1<<20)
	for s.Scan() {
		var rec SessionRecord
		if err := json.Unmarshal(s.Bytes(), &rec); err != nil {
			return nil, fmt.Errorf("decode session log: %w", err)
		}
		records = append(records, rec)
	}
	return records, s.Err()
}

type SegmentedWriter struct {
	dir         string
	base        string
	segmentSize int64
	current     *os.File
	currentIdx  int64
}

func NewSegmentedWriter(dir, base string, segmentSize int64) (*SegmentedWriter, error) {
	if segmentSize <= 0 {
		return nil, errors.New("invalid segment size")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &SegmentedWriter{dir: dir, base: base, segmentSize: segmentSize, currentIdx: -1}, nil
}

func (w *SegmentedWriter) segmentPath(index int64) string {
	return filepath.Join(w.dir, fmt.Sprintf("%s.%03d", w.base, index+1))
}

func (w *SegmentedWriter) file(index int64) (*os.File, error) {
	if w.current != nil && w.currentIdx == index {
		return w.current, nil
	}
	if w.current != nil {
		if err := w.current.Close(); err != nil {
			return nil, err
		}
		w.current = nil
		w.currentIdx = -1
	}
	path := w.segmentPath(index)
	if err := rejectNonRegularPath(path); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	w.current = f
	w.currentIdx = index
	return f, nil
}

func (w *SegmentedWriter) WriteAt(data []byte, logicalOffset int64) error {
	if logicalOffset < 0 {
		return errors.New("negative logical offset")
	}
	for len(data) > 0 {
		index := logicalOffset / w.segmentSize
		within := logicalOffset % w.segmentSize
		n := int64(len(data))
		if max := w.segmentSize - within; n > max {
			n = max
		}
		f, err := w.file(index)
		if err != nil {
			return err
		}
		wrote, err := f.WriteAt(data[:int(n)], within)
		if err != nil {
			return err
		}
		if wrote != int(n) {
			return io.ErrShortWrite
		}
		if err := f.Sync(); err != nil {
			return err
		}
		data = data[n:]
		logicalOffset += n
	}
	return nil
}

func (w *SegmentedWriter) Close() error {
	if w.current == nil {
		return nil
	}
	err := w.current.Close()
	w.current = nil
	w.currentIdx = -1
	return err
}

type ChunkSummary struct {
	Count      int
	EndOffset  int64
	MerkleRoot string
}

// AuditChunks validates the append order, every local chunk hash and the
// Merkle root while keeping only one chunk and O(log n) tree nodes in memory.
func AuditChunks(dir, base string, segmentSize int64) (ChunkSummary, error) {
	path := filepath.Join(dir, "chunks.jsonl")
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		root, _ := (&merkleAccumulator{}).Root()
		return ChunkSummary{MerkleRoot: root}, nil
	}
	if err != nil {
		return ChunkSummary{}, err
	}
	defer f.Close()
	var result ChunkSummary
	var buffer []byte
	acc := &merkleAccumulator{}
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64<<10), 1<<20)
	for s.Scan() {
		var rec ChunkRecord
		if err := json.Unmarshal(s.Bytes(), &rec); err != nil {
			return ChunkSummary{}, fmt.Errorf("decode chunk log: %w", err)
		}
		if rec.Offset != result.EndOffset || rec.Length <= 0 || rec.Length > 64<<20 {
			return ChunkSummary{}, fmt.Errorf("chunk log is non-contiguous or invalid at offset %d", rec.Offset)
		}
		if cap(buffer) < rec.Length {
			buffer = make([]byte, rec.Length)
		}
		data := buffer[:rec.Length]
		if err := readLogical(dir, base, segmentSize, rec.Offset, data); err != nil {
			return ChunkSummary{}, fmt.Errorf("read chunk at %d: %w", rec.Offset, err)
		}
		sum := sha256.Sum256(data)
		local := hex.EncodeToString(sum[:])
		if !strings.EqualFold(local, rec.SHA256) {
			return ChunkSummary{}, fmt.Errorf("chunk hash mismatch at offset %d", rec.Offset)
		}
		if err := acc.Add(rec.SHA256); err != nil {
			return ChunkSummary{}, fmt.Errorf("invalid chunk digest at offset %d: %w", rec.Offset, err)
		}
		result.Count++
		result.EndOffset += int64(rec.Length)
	}
	if err := s.Err(); err != nil {
		return ChunkSummary{}, err
	}
	result.MerkleRoot, err = acc.Root()
	return result, err
}

func VerifyChunks(dir, base string, segmentSize int64, records []ChunkRecord) error {
	for _, rec := range records {
		data := make([]byte, rec.Length)
		if err := readLogical(dir, base, segmentSize, rec.Offset, data); err != nil {
			return fmt.Errorf("read chunk at %d: %w", rec.Offset, err)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != strings.ToLower(rec.SHA256) {
			return fmt.Errorf("chunk hash mismatch at offset %d", rec.Offset)
		}
	}
	return nil
}

// VerifyExistingChunk checks an already committed logical range without
// rewriting it. It is used when a transport replays a frame after an ACK loss.
func VerifyExistingChunk(dir, base string, segmentSize, offset int64, length int, digest string) error {
	f, err := os.Open(filepath.Join(dir, "chunks.jsonl"))
	if err != nil {
		return err
	}
	defer f.Close()
	found := false
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64<<10), 1<<20)
	for s.Scan() {
		var rec ChunkRecord
		if err := json.Unmarshal(s.Bytes(), &rec); err != nil {
			return fmt.Errorf("decode chunk log: %w", err)
		}
		if rec.Offset == offset && rec.Length == length && strings.EqualFold(rec.SHA256, digest) {
			found = true
			break
		}
	}
	if err := s.Err(); err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("replayed chunk at %d is not in the verified chunk log", offset)
	}
	data := make([]byte, length)
	if err := readLogical(dir, base, segmentSize, offset, data); err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), digest) {
		return fmt.Errorf("replayed chunk at %d differs from local evidence", offset)
	}
	return nil
}

func HashLogical(dir, base string, segmentSize, total int64) (string, []FileHash, error) {
	if segmentSize <= 0 {
		return "", nil, errors.New("invalid segment size")
	}
	if total < 0 {
		return "", nil, errors.New("invalid logical size")
	}
	h := sha256.New()
	var files []FileHash
	for index, remaining := int64(0), total; remaining > 0; index++ {
		path := filepath.Join(dir, fmt.Sprintf("%s.%03d", base, index+1))
		f, err := openRegularFile(path)
		if err != nil {
			return "", nil, err
		}
		limit := segmentSize
		if remaining < limit {
			limit = remaining
		}
		fh := sha256.New()
		n, copyErr := io.CopyN(io.MultiWriter(h, fh), f, limit)
		closeErr := f.Close()
		if copyErr != nil {
			return "", nil, copyErr
		}
		if closeErr != nil {
			return "", nil, closeErr
		}
		files = append(files, FileHash{
			Path: filepath.Base(path), Size: n, SHA256: hex.EncodeToString(fh.Sum(nil)),
		})
		remaining -= n
	}
	return hex.EncodeToString(h.Sum(nil)), files, nil
}

type FileHash struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func readLogical(dir, base string, segmentSize, offset int64, out []byte) error {
	if segmentSize <= 0 {
		return errors.New("invalid segment size")
	}
	if offset < 0 {
		return errors.New("negative logical offset")
	}
	for len(out) > 0 {
		index := offset / segmentSize
		within := offset % segmentSize
		n := int64(len(out))
		if max := segmentSize - within; n > max {
			n = max
		}
		f, err := openRegularFile(filepath.Join(dir, fmt.Sprintf("%s.%03d", base, index+1)))
		if err != nil {
			return err
		}
		read, readErr := f.ReadAt(out[:int(n)], within)
		closeErr := f.Close()
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if read != int(n) {
			return io.ErrUnexpectedEOF
		}
		offset += n
		out = out[n:]
	}
	return nil
}

func rejectNonRegularPath(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing non-regular evidence path %s", path)
	}
	return nil
}

func openRegularFile(path string) (*os.File, error) {
	if err := rejectNonRegularPath(path); err != nil {
		return nil, err
	}
	return os.Open(path)
}

func MerkleRoot(records []ChunkRecord) (string, error) {
	acc := &merkleAccumulator{}
	for _, rec := range records {
		if err := acc.Add(rec.SHA256); err != nil {
			return "", fmt.Errorf("invalid chunk digest at offset %d", rec.Offset)
		}
	}
	return acc.Root()
}

type merkleAccumulator struct {
	levels   [64][sha256.Size]byte
	occupied [64]bool
	count    uint64
}

func (m *merkleAccumulator) Add(digest string) error {
	if len(digest) != 2*sha256.Size {
		return errors.New("digest is not SHA-256")
	}
	var raw [sha256.Size]byte
	n, err := hex.Decode(raw[:], []byte(digest))
	if err != nil || n != sha256.Size {
		return errors.New("digest is not SHA-256")
	}
	var leafInput [1 + sha256.Size]byte
	copy(leafInput[1:], raw[:])
	node := sha256.Sum256(leafInput[:])
	for level := 0; level < len(m.levels); level++ {
		if !m.occupied[level] {
			m.levels[level] = node
			m.occupied[level] = true
			m.count++
			return nil
		}
		node = merkleParent(m.levels[level], node)
		m.occupied[level] = false
	}
	return errors.New("too many Merkle leaves")
}

func (m *merkleAccumulator) Root() (string, error) {
	if m.count == 0 {
		sum := sha256.Sum256(nil)
		return hex.EncodeToString(sum[:]), nil
	}
	var right *[sha256.Size]byte
	for level := 0; level < len(m.levels); level++ {
		if !m.occupied[level] {
			continue
		}
		if right == nil {
			value := m.levels[level]
			right = &value
			continue
		}
		value := merkleParent(m.levels[level], *right)
		right = &value
	}
	if right == nil {
		return "", errors.New("invalid Merkle accumulator")
	}
	return hex.EncodeToString(right[:]), nil
}

func merkleParent(left, right [sha256.Size]byte) [sha256.Size]byte {
	var input [1 + 2*sha256.Size]byte
	input[0] = 1
	copy(input[1:1+sha256.Size], left[:])
	copy(input[1+sha256.Size:], right[:])
	return sha256.Sum256(input[:])
}
