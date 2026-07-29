package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/hakankrgz/imajer/internal/probe"
	"github.com/hakankrgz/imajer/internal/protocol"
	"github.com/hakankrgz/imajer/internal/source"
)

var version = "0.6.6"

func main() {
	if len(os.Args) < 2 {
		fatal(errors.New("usage: imajer-agent <probe|identify|stream|cleanup|hash|version>"))
	}
	switch os.Args[1] {
	case "probe":
		if err := json.NewEncoder(os.Stdout).Encode(probe.Collect()); err != nil {
			fatal(err)
		}
	case "stream":
		if err := stream(os.Args[2:]); err != nil {
			fatal(err)
		}
	case "identify":
		fs := flag.NewFlagSet("identify", flag.ExitOnError)
		path := fs.String("path", "", "physical disk path")
		_ = fs.Parse(os.Args[2:])
		if *path == "" {
			fatal(errors.New("identify requires --path"))
		}
		identity, err := probe.IdentifyDisk(*path)
		if err != nil {
			fatal(err)
		}
		if err := json.NewEncoder(os.Stdout).Encode(identity); err != nil {
			fatal(err)
		}
	case "hash":
		fs := flag.NewFlagSet("hash", flag.ExitOnError)
		path := fs.String("path", "", "file to hash")
		_ = fs.Parse(os.Args[2:])
		if *path == "" {
			fatal(errors.New("hash requires --path"))
		}
		f, err := os.Open(*path)
		if err != nil {
			fatal(err)
		}
		h := sha256.New()
		_, copyErr := io.Copy(h, f)
		closeErr := f.Close()
		if err := errors.Join(copyErr, closeErr); err != nil {
			fatal(err)
		}
		fmt.Println(hex.EncodeToString(h.Sum(nil)))
	case "cleanup":
		result := source.CleanupFootprint()
		if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
			fatal(err)
		}
		if len(result.Errors) > 0 {
			fatal(fmt.Errorf("cleanup completed with %d error(s)", len(result.Errors)))
		}
	case "version":
		fmt.Println(version)
	default:
		fatal(fmt.Errorf("unknown command %q", os.Args[1]))
	}
}

func stream(args []string) error {
	fs := flag.NewFlagSet("stream", flag.ContinueOnError)
	caseID := fs.String("case", "", "case ID")
	artifactID := fs.String("artifact", "", "artifact ID")
	kind := fs.String("kind", "", "disk or ram")
	path := fs.String("source", "", "read-only source path")
	sourceID := fs.String("source-id", "", "stable source identifier")
	provider := fs.String("provider", "auto", "provider")
	toolPath := fs.String("tool-path", "", "verified provider path")
	offset := fs.Int64("offset", 0, "start offset")
	size := fs.Int64("size", 0, "total source size; zero means until EOF")
	sectorSize := fs.Int64("sector-size", 512, "physical sector size")
	chunkSize := fs.Int64("chunk-size", 8<<20, "logical chunk size")
	frameSize := fs.Int64("frame-size", 0, "transport frame size; zero uses logical chunk size")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *caseID == "" || *artifactID == "" || *kind == "" || *sourceID == "" {
		return errors.New("case, artifact, kind and source-id are required")
	}
	if *offset < 0 || *size < 0 || (*size > 0 && *offset > *size) {
		return errors.New("invalid offset or size")
	}
	if *chunkSize < 1<<20 || *chunkSize > protocol.MaxPayload {
		return errors.New("chunk-size outside 1-64 MiB")
	}
	if *frameSize == 0 {
		*frameSize = *chunkSize
	}
	if *frameSize < 16<<10 || *frameSize > *chunkSize {
		return errors.New("frame-size must be between 16 KiB and logical chunk size")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	handle, err := source.Open(ctx, *kind, *provider, *path, *toolPath, *offset, *size, *sectorSize)
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = handle.Close()
		}
	}()
	sessionID, err := randomID()
	if err != nil {
		return err
	}
	enc := protocol.NewEncoder(os.Stdout)
	streamHash := sha256.New()
	buf := make([]byte, int(*chunkSize))
	current := *offset
	var seq uint64
	for {
		want := len(buf)
		if *size > 0 {
			remaining := *size - current
			if remaining == 0 {
				break
			}
			if remaining < int64(want) {
				want = int(remaining)
			}
		}
		n, readErr := io.ReadFull(handle.Reader, buf[:want])
		if errors.Is(readErr, io.ErrUnexpectedEOF) || errors.Is(readErr, io.EOF) {
			if n == 0 {
				if *size > 0 && current != *size {
					return fmt.Errorf("source ended at %d, expected %d", current, *size)
				}
				break
			}
			if *size > 0 {
				return fmt.Errorf("source ended at %d, expected %d", current+int64(n), *size)
			}
		} else if readErr != nil {
			return fmt.Errorf("read source at %d: %w", current, readErr)
		}
		chunk := buf[:n]
		sum := sha256.Sum256(chunk)
		_, _ = streamHash.Write(chunk)
		chunkDigest := hex.EncodeToString(sum[:])
		frameCount := (n + int(*frameSize) - 1) / int(*frameSize)
		readAt := time.Now().UTC()
		for frameIndex, start := 0, 0; start < n; frameIndex, start = frameIndex+1, start+int(*frameSize) {
			end := start + int(*frameSize)
			if end > n {
				end = n
			}
			payload := chunk[start:end]
			frameDigest := sha256.Sum256(payload)
			h := protocol.Header{
				Type: "data", CaseID: *caseID, ArtifactID: *artifactID, SourceID: *sourceID,
				SessionID: sessionID, Sequence: seq, Offset: current + int64(start), ReadAt: readAt,
				SHA256: hex.EncodeToString(frameDigest[:]), Provider: handle.Provider,
				ChunkOffset: current, ChunkLength: n, ChunkSHA256: chunkDigest,
				FrameIndex: frameIndex, FrameCount: frameCount,
			}
			if err := enc.Write(h, payload); err != nil {
				return fmt.Errorf("write frame: %w", err)
			}
			if err := protocol.ReadAck(os.Stdin); err != nil {
				return err
			}
			seq++
		}
		current += int64(n)
		if readErr != nil {
			break
		}
	}
	if err := handle.Close(); err != nil {
		closed = true
		return err
	}
	closed = true
	return enc.Write(protocol.Header{
		Type: "trailer", CaseID: *caseID, ArtifactID: *artifactID, SourceID: *sourceID,
		SessionID: sessionID, Sequence: seq, Offset: current, Bytes: current - *offset,
		StreamSHA256: hex.EncodeToString(streamHash.Sum(nil)), Provider: handle.Provider,
	}, nil)
}

func randomID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "imajer-agent:", err)
	os.Exit(1)
}
