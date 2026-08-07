package auth

// zbyy-compatible password encryption.
//
// The zbyy frontend (src/js/utils/common.js) encrypts the password BEFORE
// sending it to the server:
//
//   md5Pwd(password, pwdType):
//     pwdType == 2  ->  md5(password)
//     pwdType == 1  ->  md5( md5(password.toLowerCase()) + SECRET_KEY )
//                       where SECRET_KEY = "&%*$8@!!%"
//
// The server therefore NEVER sees the raw password. It stores the
// client-encrypted string as-is and compares it directly on login.
//
// This module replicates that exact algorithm so that:
//   1. The server can verify client-sent passwords.
//   2. Admin/seed tools can pre-compute hashes for seed users.
//   3. The API can optionally accept a raw password (with PwdType set) and
//      compute the hash server-side for convenience/testing.

import (
	"crypto/md5"
	"encoding/hex"
	"strings"
)

// SecretKey must match zbyy constant.js SECRET_KEY.
const SecretKey = "&%*$8@!!%"

// Md5Pwd replicates zbyy's common.md5Pwd(password, pwdType).
// pwdType 2 => md5(password); anything else => md5(md5(lower)+secret).
func Md5Pwd(password string, pwdType int) string {
	if pwdType == 2 {
		return md5Hex(password)
	}
	inner := md5Hex(strings.ToLower(password))
	return md5Hex(inner + SecretKey)
}

// Verify compares a client-sent (already-encrypted) password against the
// stored hash. Because the client already encrypted it, this is a direct
// string comparison.
func Verify(clientEncrypted, stored string) bool {
	if clientEncrypted == "" || stored == "" {
		return false
	}
	return constantTimeEq(clientEncrypted, stored)
}

// VerifyRaw computes Md5Pwd(raw, pwdType) then compares with stored.
// Useful when the API receives a raw password (e.g. admin endpoints).
func VerifyRaw(raw string, pwdType int, stored string) bool {
	return Verify(Md5Pwd(raw, pwdType), stored)
}

func md5Hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

// constantTimeEq prevents timing attacks on password comparison.
func constantTimeEq(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var r byte
	for i := 0; i < len(a); i++ {
		r |= a[i] ^ b[i]
	}
	return r == 0
}
