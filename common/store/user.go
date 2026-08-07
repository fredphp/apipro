package store

// Redis-backed user store. Users are persisted in Redis so the service is stateless.
// Keys:
//   apipro:user:uid:<uid>          -> JSON of public user record (NO password)
//   apipro:user:pwd:<uid>          -> bcrypt password hash
//   apipro:user:phone:<cc>:<phone> -> uid lookup
//   apipro:user:loginname:<name>   -> uid lookup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"apipro/pkg/fixture"

	"github.com/zeromicro/go-zero/core/stores/redis"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserExists      = errors.New("user already exists")
	ErrUserNotFound    = errors.New("user not found")
	ErrInvalidPassword = errors.New("invalid password")
)

type UserStore struct {
	rdb *redis.Redis
}

func NewUserStore(rdb *redis.Redis) *UserStore {
	return &UserStore{rdb: rdb}
}

func phoneKey(cc, phone string) string { return "apipro:user:phone:" + cc + ":" + phone }
func loginNameKey(name string) string  { return "apipro:user:loginname:" + strings.ToLower(name) }
func uidKey(uid string) string         { return "apipro:user:uid:" + uid }
func pwdKey(uid string) string         { return "apipro:user:pwd:" + uid }

// Register creates a new user.
func (s *UserStore) Register(ctx context.Context, loginName, cc, phone, password string) (fixture.UserRecord, error) {
	loginName = strings.TrimSpace(loginName)
	phone = strings.TrimSpace(phone)
	cc = strings.TrimSpace(cc)
	if loginName == "" || phone == "" || cc == "" || password == "" {
		return fixture.UserRecord{}, errors.New("missing fields")
	}
	if len(password) < 6 || len(password) > 64 {
		return fixture.UserRecord{}, errors.New("password length must be 6-64")
	}
	if exists, _ := s.rdb.Exists(phoneKey(cc, phone)); exists {
		return fixture.UserRecord{}, ErrUserExists
	}
	if exists, _ := s.rdb.Exists(loginNameKey(loginName)); exists {
		return fixture.UserRecord{}, ErrUserExists
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return fixture.UserRecord{}, err
	}
	uid := fixture.GenUid("U")
	rec := fixture.UserRecord{
		Uid: uid, LoginName: loginName, NickName: loginName,
		Phone: phone, CountryCode: cc, Password: "",
		Grow: 0, Score: 0, Level: 1,
		Avatar:  "https://cdn.zbyy.example/avatar/default.png",
		IsUser:  1, CreatedAt: nowUnix(),
	}
	// store public record (no password) + password hash separately
	b, _ := json.Marshal(rec)
	if err := s.rdb.Set(uidKey(uid), string(b)); err != nil {
		return fixture.UserRecord{}, err
	}
	_ = s.rdb.Set(pwdKey(uid), string(hash))
	_ = s.rdb.Set(phoneKey(cc, phone), uid)
	_ = s.rdb.Set(loginNameKey(loginName), uid)
	return rec, nil
}

// Login validates credentials.
func (s *UserStore) Login(ctx context.Context, cc, phone, password string) (fixture.UserRecord, error) {
	uid, err := s.rdb.Get(phoneKey(cc, phone))
	if err != nil || uid == "" {
		return fixture.UserRecord{}, ErrUserNotFound
	}
	rec, err := s.getByUid(uid)
	if err != nil {
		return fixture.UserRecord{}, err
	}
	hash, err := s.rdb.Get(pwdKey(uid))
	if err != nil || hash == "" {
		return fixture.UserRecord{}, ErrInvalidPassword
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return fixture.UserRecord{}, ErrInvalidPassword
	}
	return rec, nil
}

// GetByUid fetches a user by uid.
func (s *UserStore) GetByUid(ctx context.Context, uid string) (fixture.UserRecord, error) {
	return s.getByUid(uid)
}

func (s *UserStore) getByUid(uid string) (fixture.UserRecord, error) {
	raw, err := s.rdb.Get(uidKey(uid))
	if err != nil || raw == "" {
		return fixture.UserRecord{}, ErrUserNotFound
	}
	var rec fixture.UserRecord
	if err := json.Unmarshal([]byte(raw), &rec); err != nil {
		return fixture.UserRecord{}, err
	}
	return rec, nil
}

// CreateGuest creates a transient guest user (24h TTL).
func (s *UserStore) CreateGuest(ctx context.Context) (fixture.UserRecord, error) {
	uid := fixture.GenUid("G")
	rec := fixture.UserRecord{
		Uid: uid, LoginName: "", NickName: "游客" + uid[len(uid)-4:],
		Phone: "", CountryCode: "", Password: "",
		Grow: 0, Score: 0, Level: 1,
		Avatar:  "https://cdn.zbyy.example/avatar/guest.png",
		IsUser:  0, CreatedAt: nowUnix(),
	}
	b, _ := json.Marshal(rec)
	_ = s.rdb.Setex(uidKey(uid), string(b), 24*3600)
	return rec, nil
}

func nowUnix() int64 { return fixture.NowUnix() }

// DebugString for logging
func (s *UserStore) DebugString(rec fixture.UserRecord) string {
	return fmt.Sprintf("uid=%s nick=%s isUser=%d", rec.Uid, rec.NickName, rec.IsUser)
}
