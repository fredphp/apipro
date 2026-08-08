// Package codec implements the AES-128-ECB/PKCS7 transport used by the
// YuYanTV (zbyy) frontend client. Wire format on the wire is:
//
//	[0x00][0xA0][uint32_be(ciphertext_len)][ciphertext...]
//
// The ciphertext is AES-128-ECB-encrypted (NO IV) using a 16-byte ASCII key.
// Padding is PKCS7. Plaintext is the raw UTF-8 JSON of the business payload
// (we skip the legacy protobuf envelope for simplicity — the encrypted blob
// decrypts directly to JSON, which the application reads/writes).
//
// Two keys are used in production:
//   - RequestKey  (default "PHp1st5vEg5Ca8FH") — decrypt client→server bodies
//   - ResponseKey (default "qlCJekfRKwWkQxl7") — encrypt server→client bodies
//
// Verified test vector from backend-zero:
//	key        = "PHp1st5vEg5Ca8FH" (16 bytes)
//	plaintext  = `{"a":1}` (7 bytes)
//	ciphertext = hex f6d9666260c649f41050f460aa0a2cbe
package codec

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
)

// Default keys — match backend-zero production values.
const (
	DefaultRequestKey  = "PHp1st5vEg5Ca8FH"
	DefaultResponseKey = "qlCJekfRKwWkQxl7"

	frameByte0     = 0x00
	frameByte1     = 0xA0
	frameHeaderLen = 6 // 2 magic + 4 length
)

var (
	ErrInvalidKey       = errors.New("codec: AES-128 key must be 16 bytes")
	ErrInvalidFrame     = errors.New("codec: invalid frame header")
	ErrLengthMismatch   = errors.New("codec: frame length mismatch")
	ErrPadding          = errors.New("codec: invalid PKCS7 padding")
	ErrEmpty            = errors.New("codec: empty ciphertext")
)

// Encrypt applies AES-128-ECB/PKCS7 to plaintext using the supplied 16-byte key.
func Encrypt(plaintext, key []byte) ([]byte, error) {
	if len(key) != 16 {
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("codec: new cipher: %w", err)
	}
	padded := pkcs7Pad(plaintext, block.BlockSize())
	if len(padded) == 0 {
		return nil, ErrEmpty
	}
	out := make([]byte, len(padded))
	ecbEncrypt(block, out, padded)
	return out, nil
}

// Decrypt reverses AES-128-ECB/PKCS7.
func Decrypt(ciphertext, key []byte) ([]byte, error) {
	if len(key) != 16 {
		return nil, ErrInvalidKey
	}
	if len(ciphertext) == 0 {
		return nil, ErrEmpty
	}
	if len(ciphertext)%16 != 0 {
		return nil, fmt.Errorf("codec: ciphertext not multiple of block size (%d)", len(ciphertext))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("codec: new cipher: %w", err)
	}
	out := make([]byte, len(ciphertext))
	ecbDecrypt(block, out, ciphertext)
	return pkcs7Unpad(out, block.BlockSize())
}

// EncodeFrame encrypts payload then prepends the 6-byte frame header.
//   bytes[0..1] = 0x00 0xA0
//   bytes[2..5] = BE uint32 ciphertext length
//   bytes[6..]  = ciphertext
func EncodeFrame(payload, key []byte) ([]byte, error) {
	ct, err := Encrypt(payload, key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, frameHeaderLen+len(ct))
	out[0] = frameByte0
	out[1] = frameByte1
	binary.BigEndian.PutUint32(out[2:6], uint32(len(ct)))
	copy(out[6:], ct)
	return out, nil
}

// DecodeFrame strips + validates the 6-byte header then decrypts.
// Returns the plaintext JSON payload.
func DecodeFrame(raw, key []byte) ([]byte, error) {
	if len(raw) < frameHeaderLen {
		return nil, ErrInvalidFrame
	}
	if raw[0] != frameByte0 || raw[1] != frameByte1 {
		return nil, ErrInvalidFrame
	}
	n := binary.BigEndian.Uint32(raw[2:6])
	if int(n) != len(raw)-frameHeaderLen {
		return nil, ErrLengthMismatch
	}
	return Decrypt(raw[frameHeaderLen:], key)
}

// ----- ECB block processing (Go's stdlib has no ECB mode) -----

func ecbEncrypt(block cipher.Block, dst, src []byte) {
	bs := block.BlockSize()
	for i := 0; i < len(src); i += bs {
		block.Encrypt(dst[i:i+bs], src[i:i+bs])
	}
}

func ecbDecrypt(block cipher.Block, dst, src []byte) {
	bs := block.BlockSize()
	for i := 0; i < len(src); i += bs {
		block.Decrypt(dst[i:i+bs], src[i:i+bs])
	}
}

// ----- PKCS7 -----

func pkcs7Pad(data []byte, blockSize int) []byte {
	if blockSize <= 0 || blockSize > 255 {
		return nil
	}
	pad := blockSize - (len(data) % blockSize)
	out := make([]byte, len(data)+pad)
	copy(out, data)
	for i := len(data); i < len(out); i++ {
		out[i] = byte(pad)
	}
	return out
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	n := len(data)
	if n == 0 || n%blockSize != 0 {
		return nil, ErrPadding
	}
	pad := int(data[n-1])
	if pad == 0 || pad > blockSize || pad > n {
		return nil, ErrPadding
	}
	// constant-time pad verification
	var v byte
	for i := n - pad; i < n; i++ {
		v |= data[i] ^ byte(pad)
	}
	if subtle.ConstantTimeByteEq(v, 0) != 1 {
		return nil, ErrPadding
	}
	return data[:n-pad], nil
}
