package protocol

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

const (
	Version              = 1
	MaxHeaderLength      = 64 << 10
	MaxPayload           = 64 << 20
	Ack             byte = 0x06
	Nack            byte = 0x15
)

type Header struct {
	Version      int       `json:"version"`
	Type         string    `json:"type"`
	CaseID       string    `json:"case_id,omitempty"`
	ArtifactID   string    `json:"artifact_id,omitempty"`
	SourceID     string    `json:"source_id,omitempty"`
	SessionID    string    `json:"session_id,omitempty"`
	Sequence     uint64    `json:"sequence,omitempty"`
	Offset       int64     `json:"offset,omitempty"`
	Length       int       `json:"length,omitempty"`
	ChunkOffset  int64     `json:"chunk_offset,omitempty"`
	ChunkLength  int       `json:"chunk_length,omitempty"`
	ChunkSHA256  string    `json:"chunk_sha256,omitempty"`
	FrameIndex   int       `json:"frame_index,omitempty"`
	FrameCount   int       `json:"frame_count,omitempty"`
	ReadAt       time.Time `json:"read_at,omitempty"`
	SHA256       string    `json:"sha256,omitempty"`
	StreamSHA256 string    `json:"stream_sha256,omitempty"`
	Bytes        int64     `json:"bytes,omitempty"`
	Provider     string    `json:"provider,omitempty"`
	Message      string    `json:"message,omitempty"`
}

type Frame struct {
	Header  Header
	Payload []byte
}

type Encoder struct {
	w *bufio.Writer
}

func NewEncoder(w io.Writer) *Encoder { return &Encoder{w: bufio.NewWriterSize(w, 128<<10)} }

func (e *Encoder) Write(h Header, payload []byte) error {
	if h.Version == 0 {
		h.Version = Version
	}
	if len(payload) > MaxPayload {
		return fmt.Errorf("payload %d exceeds maximum", len(payload))
	}
	h.Length = len(payload)
	raw, err := json.Marshal(h)
	if err != nil {
		return fmt.Errorf("marshal frame header: %w", err)
	}
	if len(raw) > MaxHeaderLength {
		return errors.New("frame header too large")
	}
	if err := binary.Write(e.w, binary.BigEndian, uint32(len(raw))); err != nil {
		return err
	}
	if _, err := e.w.Write(raw); err != nil {
		return err
	}
	if _, err := e.w.Write(payload); err != nil {
		return err
	}
	return e.w.Flush()
}

type Decoder struct {
	r *bufio.Reader
}

func NewDecoder(r io.Reader) *Decoder { return &Decoder{r: bufio.NewReaderSize(r, 128<<10)} }

func (d *Decoder) Read() (*Frame, error) {
	var n uint32
	if err := binary.Read(d.r, binary.BigEndian, &n); err != nil {
		return nil, err
	}
	if n == 0 || n > MaxHeaderLength {
		return nil, fmt.Errorf("invalid header length %d", n)
	}
	raw := make([]byte, n)
	if _, err := io.ReadFull(d.r, raw); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	var h Header
	if err := json.Unmarshal(raw, &h); err != nil {
		return nil, fmt.Errorf("decode header: %w", err)
	}
	if h.Version != Version {
		return nil, fmt.Errorf("unsupported protocol version %d", h.Version)
	}
	if h.Length < 0 || h.Length > MaxPayload {
		return nil, fmt.Errorf("invalid payload length %d", h.Length)
	}
	payload := make([]byte, h.Length)
	if _, err := io.ReadFull(d.r, payload); err != nil {
		return nil, fmt.Errorf("read payload: %w", err)
	}
	return &Frame{Header: h, Payload: payload}, nil
}

func WriteAck(w io.Writer, ok bool) error {
	b := Ack
	if !ok {
		b = Nack
	}
	_, err := w.Write([]byte{b})
	return err
}

func ReadAck(r io.Reader) error {
	var b [1]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return err
	}
	if b[0] != Ack {
		return errors.New("controller rejected chunk")
	}
	return nil
}
