package auth

import (
	"encoding/hex"
	"testing"
)

func TestMD5Vector(t *testing.T) {
	got := MD5Hex("qwe123")
	want := "200820e3227815ed1756a6b531e7e0d2"
	if got != want {
		t.Fatalf("md5 mismatch: got %s want %s", got, want)
	}
}

func TestDBPasswordVector(t *testing.T) {
	cases := []struct {
		clientMd5, salt, want string
	}{
		{"200820e3227815ed1756a6b531e7e0d2", "7Whd1U2T1pjeDP4HcSVDxwBMF5Vf6NWx", "8ec733b6de4825a437faee2c01ddd309"},
		{"200820e3227815ed1756a6b531e7e0d2", "ZHk1t4TvnRzWhXDa5vWFWtzU5R7K82qv", "ebde8759265babd3f193ecf67223b157"},
	}
	for _, c := range cases {
		got := DBPassword(c.clientMd5, c.salt)
		if got != c.want {
			t.Fatalf("db password mismatch for salt=%s: got %s want %s", c.salt, got, c.want)
		}
	}
}

func TestHashAndVerify(t *testing.T) {
	clientMd5 := MD5Hex("mySecret123")
	stored, salt, err := HashPassword(clientMd5)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if len(salt) != 44 {
		t.Fatalf("salt length: got %d want 44", len(salt))
	}
	if !VerifyPassword(clientMd5, stored, salt) {
		t.Fatalf("verify should succeed for correct password")
	}
	if VerifyPassword(MD5Hex("wrong"), stored, salt) {
		t.Fatalf("verify should fail for wrong password")
	}
}

func TestPwdType1Rejected(t *testing.T) {
	_, err := ClientPassword("abc", 1)
	if err != ErrUnsupportedPwdType {
		t.Fatalf("expected ErrUnsupportedPwdType, got %v", err)
	}
}

func TestSaltRandomness(t *testing.T) {
	a, _ := RandomSalt()
	b, _ := RandomSalt()
	if a == b {
		t.Fatalf("salts should differ")
	}
	if hex.EncodeToString([]byte(a)) == hex.EncodeToString([]byte(b)) {
		t.Fatalf("salts should differ")
	}
}
