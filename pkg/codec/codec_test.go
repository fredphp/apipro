package codec

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// Verify the documented test vector from backend-zero.
func TestEncryptVector(t *testing.T) {
	key := []byte("PHp1st5vEg5Ca8FH")
	plain := []byte(`{"a":1}`)
	want, _ := hex.DecodeString("f6d9666260c649f41050f460aa0a2cbe")
	ct, err := Encrypt(plain, key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !bytes.Equal(ct, want) {
		t.Fatalf("ciphertext mismatch: got %x want %x", ct, want)
	}
	// round-trip
	pt, err := Decrypt(ct, key)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(pt, plain) {
		t.Fatalf("plaintext mismatch: got %q want %q", pt, plain)
	}
}

func TestFrameRoundTrip(t *testing.T) {
	key := []byte("qlCJekfRKwWkQxl7")
	plain := []byte(`{"hello":"world","n":42}`)
	fr, err := EncodeFrame(plain, key)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if fr[0] != 0x00 || fr[1] != 0xA0 {
		t.Fatalf("bad magic: %x %x", fr[0], fr[1])
	}
	pt, err := DecodeFrame(fr, key)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !bytes.Equal(pt, plain) {
		t.Fatalf("round trip mismatch: got %q want %q", pt, plain)
	}
}

func TestInvalidKeyRejected(t *testing.T) {
	_, err := Encrypt([]byte("x"), []byte("short"))
	if err != ErrInvalidKey {
		t.Fatalf("expected ErrInvalidKey, got %v", err)
	}
}

func TestInvalidFrame(t *testing.T) {
	_, err := DecodeFrame([]byte{0x01, 0x02, 0x00, 0x00, 0x00, 0x00}, []byte("PHp1st5vEg5Ca8FH"))
	if err != ErrInvalidFrame {
		t.Fatalf("expected ErrInvalidFrame, got %v", err)
	}
}
