package auth

// Password hashing matching backend-zero.
//
// Algorithm (audience accounts, pwd_type=2 ONLY):
//
//	client_pwd = lowercase_md5_hex(plain_password)   // CASE-SENSITIVE on input; md5 output is lowercase
//	db_password = lowercase_md5_hex(client_pwd + salt)
//	salt = base64.StdEncoding(32 random bytes) = 44 ASCII chars
//
// pwd_type=1 is UNSUPPORTED (returns ErrUnsupportedPwdType).
//
// Test vectors (verified against backend-zero password_test.go):
//	MD5Hex("qwe123") = "200820e3227815ed1756a6b531e7e0d2"
//	DBPassword("200820e3227815ed1756a6b531e7e0d2", "7Whd1U2T1pjeDP4HcSVDxwBMF5Vf6NWx")
//	  = "8ec733b6de4825a437faee2c01ddd309"

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
)

const (
	PwdTypeMD5 = 2 // pwd_type=2 is the only supported type
)

// Errors
var (
	ErrUnsupportedPwdType = errors.New("auth: pwd_type=1 is unsupported; use pwd_type=2")
	ErrEmptyPassword      = errors.New("auth: empty password")
)

// MD5Hex returns the lowercase hex md5 of the UTF-8 bytes of s.
func MD5Hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

// ClientPassword replicates the client-side pre-hash. pwd_type=2 returns
// MD5Hex(plain); pwd_type=1 returns ErrUnsupportedPwdType.
func ClientPassword(plain string, pwdType int) (string, error) {
	if pwdType != PwdTypeMD5 {
		return "", ErrUnsupportedPwdType
	}
	if plain == "" {
		return "", ErrEmptyPassword
	}
	return MD5Hex(plain), nil
}

// HashPassword takes the client-pre-hashed md5 (what the client sends as
// `password`) and returns (dbPassword, salt, err). The salt is 44-char base64
// of 32 random bytes. dbPassword = MD5Hex(clientMd5 + salt).
//
// plainMd5 is what the client sent (already md5 of the raw password).
// It is NOT re-hashed.
func HashPassword(plainMd5 string) (storedHash, salt string, err error) {
	if plainMd5 == "" {
		return "", "", ErrEmptyPassword
	}
	salt, err = RandomSalt()
	if err != nil {
		return "", "", err
	}
	storedHash = DBPassword(plainMd5, salt)
	return storedHash, salt, nil
}

// DBPassword = MD5Hex(clientMd5 + salt). Salt is appended DIRECTLY.
func DBPassword(clientMd5, salt string) string {
	return MD5Hex(clientMd5 + salt)
}

// VerifyPassword compares a client-sent md5 against the stored hash + salt.
// constant-time on the final comparison.
func VerifyPassword(plainMd5, storedHash, salt string) bool {
	if plainMd5 == "" || storedHash == "" {
		return false
	}
	computed := DBPassword(plainMd5, salt)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(storedHash)) == 1
}

// RandomSalt = base64(32 random bytes) — 44-char ASCII string.
func RandomSalt() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw[:]), nil
}
