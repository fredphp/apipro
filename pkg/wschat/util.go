package wschat

// Frame body encryption / decryption helpers.
//
// The body is AES-128-ECB-PKCS7 encrypted using the same key as the HTTP
// transport (RequestKey for client→server decrypt; ResponseKey for
// server→client encrypt).

import (
	"apipro/pkg/codec"
)

// encryptFrameBody applies AES-128-ECB/PKCS7 to the body.
// (No framing header — the WS frame header already carries the length.)
func encryptFrameBody(body, key []byte) ([]byte, error) {
	return codec.Encrypt(body, key)
}

// decryptFrameBody reverses AES-128-ECB/PKCS7.
func decryptFrameBody(body, key []byte) ([]byte, error) {
	return codec.Decrypt(body, key)
}
