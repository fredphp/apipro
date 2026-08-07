package jwtx

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	ClaimUid      = "uid"
	ClaimNickName = "nickName"
	ClaimIsUser   = "isUser"
	ClaimTyp      = "typ"
	TypAccess     = "access"
	TypRefresh    = "refresh"
)

type Claims struct {
	Uid      string `json:"uid"`
	NickName string `json:"nickName"`
	IsUser   int32  `json:"isUser"`
	Typ      string `json:"typ"`
	jwt.RegisteredClaims
}

// Sign issues a token.
func Sign(secret string, uid, nickName string, isUser int32, typ string, ttl time.Duration) (string, int64, error) {
	exp := time.Now().Add(ttl)
	claims := Claims{
		Uid:      uid,
		NickName: nickName,
		IsUser:   isUser,
		Typ:      typ,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   uid,
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := t.SignedString([]byte(secret))
	return s, exp.Unix(), err
}

// Verify validates and parses the token.
func Verify(secret, tokenStr string) (*Claims, error) {
	c := &Claims{}
	tok, err := jwt.ParseWithClaims(tokenStr, c, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	if !tok.Valid {
		return nil, errors.New("invalid token")
	}
	return c, nil
}
