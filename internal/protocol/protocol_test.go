package protocol

import (
	"bytes"
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	var b bytes.Buffer
	e := NewEncoder(&b)
	want := Header{Type: "data", Offset: 42, SHA256: "abc", ReadAt: time.Now().UTC().Truncate(time.Nanosecond)}
	if err := e.Write(want, []byte("evidence")); err != nil {
		t.Fatal(err)
	}
	got, err := NewDecoder(&b).Read()
	if err != nil {
		t.Fatal(err)
	}
	if got.Header.Offset != want.Offset || string(got.Payload) != "evidence" {
		t.Fatalf("unexpected frame: %#v %q", got.Header, got.Payload)
	}
}

func TestRejectOversizedHeader(t *testing.T) {
	var b bytes.Buffer
	b.Write([]byte{0, 1, 0, 1})
	if _, err := NewDecoder(&b).Read(); err == nil {
		t.Fatal("expected error")
	}
}

func FuzzDecodeFrame(f *testing.F) {
	var seed bytes.Buffer
	if err := NewEncoder(&seed).Write(Header{Type: "data", CaseID: "CASE", Offset: 0}, []byte("seed")); err != nil {
		f.Fatal(err)
	}
	f.Add(seed.Bytes())
	f.Add([]byte{0, 0, 0, 1, '{'})
	f.Fuzz(func(t *testing.T, raw []byte) {
		frame, err := NewDecoder(bytes.NewReader(raw)).Read()
		if err != nil {
			return
		}
		if frame.Header.Version != Version || len(frame.Payload) > MaxPayload {
			t.Fatalf("decoder accepted an invalid frame: %#v", frame.Header)
		}
	})
}
