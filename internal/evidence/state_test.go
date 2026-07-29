package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSegmentedWriterAndVerify(t *testing.T) {
	dir := t.TempDir()
	w, err := NewSegmentedWriter(dir, "disk", 16)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("0123456789abcdefghij")
	if err := w.WriteAt(data, 8); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir + "/disk.002"); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	recs := []ChunkRecord{{Offset: 8, Length: len(data), SHA256: hex.EncodeToString(sum[:]), ReadAt: time.Now()}}
	if err := VerifyChunks(dir, "disk", 16, recs); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "disk.001"), os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt([]byte{0xff}, 8); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := VerifyChunks(dir, "disk", 16, recs); err == nil {
		t.Fatal("one-byte evidence corruption was not detected")
	}
}

func TestHashLogicalRejectsInvalidSegmentSize(t *testing.T) {
	if _, _, err := HashLogical(t.TempDir(), "disk", 0, 1); err == nil {
		t.Fatal("zero segment size was accepted")
	}
}

func TestSegmentedWriterRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "outside")
	if err := os.WriteFile(target, []byte("must remain unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "disk.001")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}
	writer, err := NewSegmentedWriter(dir, "disk", 16)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if err := writer.WriteAt([]byte("forensic"), 0); err == nil {
		t.Fatal("symlink evidence segment was accepted")
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "must remain unchanged" {
		t.Fatal("symlink target was modified")
	}
}

func TestMerkleRootStable(t *testing.T) {
	a := sha256.Sum256([]byte("a"))
	b := sha256.Sum256([]byte("b"))
	recs := []ChunkRecord{{SHA256: hex.EncodeToString(a[:])}, {SHA256: hex.EncodeToString(b[:])}}
	x, err := MerkleRoot(recs)
	if err != nil {
		t.Fatal(err)
	}
	y, _ := MerkleRoot(recs)
	if x != y || x == "" {
		t.Fatal("unstable merkle root")
	}
}

func TestStreamingMerkleMatchesReferenceForOddAndEvenLeafCounts(t *testing.T) {
	for count := 0; count <= 100; count++ {
		records := make([]ChunkRecord, 0, count)
		for i := 0; i < count; i++ {
			sum := sha256.Sum256([]byte(fmt.Sprintf("leaf-%d", i)))
			records = append(records, ChunkRecord{Offset: int64(i), SHA256: hex.EncodeToString(sum[:])})
		}
		got, err := MerkleRoot(records)
		if err != nil {
			t.Fatal(err)
		}
		want := referenceMerkleRoot(t, records)
		if got != want {
			t.Fatalf("count=%d got=%s want=%s", count, got, want)
		}
	}
}

func referenceMerkleRoot(t *testing.T, records []ChunkRecord) string {
	t.Helper()
	if len(records) == 0 {
		sum := sha256.Sum256(nil)
		return hex.EncodeToString(sum[:])
	}
	level := make([][sha256.Size]byte, 0, len(records))
	for _, rec := range records {
		raw, err := hex.DecodeString(rec.SHA256)
		if err != nil {
			t.Fatal(err)
		}
		input := append([]byte{0}, raw...)
		level = append(level, sha256.Sum256(input))
	}
	for len(level) > 1 {
		next := make([][sha256.Size]byte, 0, (len(level)+1)/2)
		for i := 0; i < len(level); i += 2 {
			if i+1 == len(level) {
				next = append(next, level[i])
			} else {
				next = append(next, merkleParent(level[i], level[i+1]))
			}
		}
		level = next
	}
	return hex.EncodeToString(level[0][:])
}

func TestSegmentAddressingBeyondTwoTiBUsesConstantMemory(t *testing.T) {
	dir := t.TempDir()
	const segmentSize = int64(2 << 30)
	const offset = int64(2<<40) + 123
	w, err := NewSegmentedWriter(dir, "disk", segmentSize)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.WriteAt([]byte{0x5a}, offset); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	index := offset / segmentSize
	path := filepath.Join(dir, "disk."+formatSegment(index+1))
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != offset%segmentSize+1 {
		t.Fatalf("unexpected sparse segment size: %d", info.Size())
	}
}

func TestSegmentedWriterKeepsOnlyOneFileOpen(t *testing.T) {
	dir := t.TempDir()
	w, err := NewSegmentedWriter(dir, "disk", 16)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.WriteAt(make([]byte, 48), 0); err != nil {
		t.Fatal(err)
	}
	if w.current == nil || w.currentIdx != 2 {
		t.Fatalf("writer did not bound open files: current=%v index=%d", w.current, w.currentIdx)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkStreamingMerkleTwoTiBLogicalImage(b *testing.B) {
	// 2 TiB / 8 MiB = 262,144 chunk leaves. The accumulator retains only
	// 64 fixed SHA-256 nodes regardless of logical evidence size.
	sum := sha256.Sum256([]byte("synthetic-8MiB-chunk"))
	digest := hex.EncodeToString(sum[:])
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		acc := &merkleAccumulator{}
		for i := 0; i < 262144; i++ {
			if err := acc.Add(digest); err != nil {
				b.Fatal(err)
			}
		}
		if _, err := acc.Root(); err != nil {
			b.Fatal(err)
		}
	}
}

func formatSegment(index int64) string {
	return fmt.Sprintf("%03d", index)
}
