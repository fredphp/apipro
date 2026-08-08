package auth

// Opaque-token session store backed by Redis.
//
// Tokens are random hex strings (NOT JWT):
//   - AccessToken  = hex(rand 24 bytes) = 48 hex chars, TTL 30min
//   - RefreshToken = hex(rand 32 bytes) = 64 hex chars, TTL 30 days
//   - Guest AccessToken TTL = 24h, no refresh
//
// Redis key patterns:
//   yuyan:sess:v2:<accessToken>      → JSON session record (TTL = TTL)
//   yuyan:refresh:v2:<refreshToken>  → accessToken (for rotation)
//
// Same-device kick-out is omitted in this lightweight rebuild (the production
// backend-zero uses yuyan:dev:v2:<uid>:<device> to enforce single-session per
// device). For the apipro rebuild, the latest login simply wins.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

const (
	sessKeyPrefix    = "yuyan:sess:v2:"
	refreshKeyPrefix = "yuyan:refresh:v2:"

	DefaultUserAccessTTL  = 30 * time.Minute
	DefaultUserRefreshTTL = 30 * 24 * time.Hour
	DefaultGuestAccessTTL = 24 * time.Hour
)

// Session is the data stored per access token.
type Session struct {
	AccessToken    string `json:"accessToken"`
	RefreshToken   string `json:"refreshToken,omitempty"`
	UID            int64  `json:"uid"`
	NickName       string `json:"nickName"`
	Icon           string `json:"icon"`
	UserType       int    `json:"userType"`     // 1=audience, 2=anchor, 3=admin
	DeviceType     string `json:"deviceType"`   // android|ios|pc|h5
	Plat           int    `json:"plat"`         // 1=android 2=ios 3=pc 4=h5
	IsGuest        bool   `json:"isGuest"`
	AccessExpireAt int64  `json:"accessExpireAt"`   // unix seconds
	RefreshExpireAt int64 `json:"refreshExpireAt,omitempty"` // unix seconds; 0 for guest
}

// Errors
var (
	ErrSessionNotFound = errors.New("auth: session not found")
	ErrRefreshDenied   = errors.New("auth: refresh denied (guest or expired)")
	ErrUserBanned      = errors.New("auth: user banned")
)

// SessionStore manages opaque tokens in Redis.
type SessionStore struct {
	rdb            *redis.Redis
	userAccessTTL  time.Duration
	userRefreshTTL time.Duration
	guestAccessTTL time.Duration
}

// NewSessionStore builds a SessionStore. nil rdb = in-memory fallback (not
// implemented here; callers should ensure Redis is available).
func NewSessionStore(rdb *redis.Redis) *SessionStore {
	return &SessionStore{
		rdb:            rdb,
		userAccessTTL:  DefaultUserAccessTTL,
		userRefreshTTL: DefaultUserRefreshTTL,
		guestAccessTTL: DefaultGuestAccessTTL,
	}
}

// IssueUser creates a new user session. Returns the populated Session (with
// AccessToken + RefreshToken filled in).
func (s *SessionStore) IssueUser(ctx context.Context, uid int64, nickName, icon string, userType int, plat int) (*Session, error) {
	access, err := randomHex(24) // 48 hex chars
	if err != nil {
		return nil, err
	}
	refresh, err := randomHex(32) // 64 hex chars
	if err != nil {
		return nil, err
	}
	now := time.Now()
	sess := &Session{
		AccessToken:     access,
		RefreshToken:    refresh,
		UID:             uid,
		NickName:        nickName,
		Icon:            icon,
		UserType:        userType,
		DeviceType:      platToDevice(plat),
		Plat:            plat,
		IsGuest:         false,
		AccessExpireAt:  now.Add(s.userAccessTTL).Unix(),
		RefreshExpireAt: now.Add(s.userRefreshTTL).Unix(),
	}
	if err := s.persist(ctx, sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// IssueGuest creates a guest session (uid=0, no refresh, 24h TTL).
func (s *SessionStore) IssueGuest(ctx context.Context, plat int) (*Session, error) {
	access, err := randomHex(24)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	sess := &Session{
		AccessToken:    access,
		UID:            0,
		NickName:       GuestNickName(),
		Icon:           "",
		UserType:       1,
		DeviceType:     platToDevice(plat),
		Plat:           plat,
		IsGuest:        true,
		AccessExpireAt: now.Add(s.guestAccessTTL).Unix(),
	}
	if err := s.persist(ctx, sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// Refresh rotates the access+refresh tokens for the given refresh token.
// Returns ErrRefreshDenied for guests, missing, or expired refresh.
func (s *SessionStore) Refresh(ctx context.Context, refreshToken string, userStatusFn func(uid int64) (int, error)) (*Session, error) {
	if s.rdb == nil || refreshToken == "" {
		return nil, ErrRefreshDenied
	}
	// 1. Look up access token via refresh key.
	access, err := s.rdb.Get(refreshKeyPrefix + refreshToken)
	if err != nil || access == "" {
		return nil, ErrRefreshDenied
	}
	// 2. Load the session.
	old, err := s.Get(ctx, access)
	if err != nil {
		return nil, ErrRefreshDenied
	}
	if old.IsGuest {
		return nil, ErrRefreshDenied
	}
	// 3. Check refresh expiry.
	if old.RefreshExpireAt > 0 && time.Now().Unix() > old.RefreshExpireAt {
		_ = s.revoke(ctx, access, old.RefreshToken)
		return nil, ErrRefreshDenied
	}
	// 4. Check user status (banned?).
	if userStatusFn != nil {
		st, err := userStatusFn(old.UID)
		if err != nil || st != 1 {
			_ = s.RevokeAllForUID(ctx, old.UID)
			return nil, ErrUserBanned
		}
	}
	// 5. Delete old, issue new (rotation; refresh window NOT extended).
	_ = s.revoke(ctx, access, old.RefreshToken)
	return s.IssueUser(ctx, old.UID, old.NickName, old.Icon, old.UserType, old.Plat)
}

// Get loads a session by access token. Returns ErrSessionNotFound if missing
// or expired.
func (s *SessionStore) Get(ctx context.Context, accessToken string) (*Session, error) {
	if s.rdb == nil || accessToken == "" {
		return nil, ErrSessionNotFound
	}
	raw, err := s.rdb.Get(sessKeyPrefix + accessToken)
	if err != nil || raw == "" {
		return nil, ErrSessionNotFound
	}
	var sess Session
	if err := json.Unmarshal([]byte(raw), &sess); err != nil {
		return nil, ErrSessionNotFound
	}
	// Expiry check (Redis TTL should handle, but double-check).
	if sess.AccessExpireAt > 0 && time.Now().Unix() > sess.AccessExpireAt {
		return nil, ErrSessionNotFound
	}
	return &sess, nil
}

// Revoke deletes a session by access token (logout).
func (s *SessionStore) Revoke(ctx context.Context, accessToken string) error {
	if s.rdb == nil || accessToken == "" {
		return nil
	}
	sess, err := s.Get(ctx, accessToken)
	if err != nil {
		// already gone — fine
		return nil
	}
	return s.revoke(ctx, accessToken, sess.RefreshToken)
}

// RevokeAllForUID iterates device keys — for the lightweight rebuild we just
// delete the current access token (single-device tracking omitted).
func (s *SessionStore) RevokeAllForUID(ctx context.Context, uid int64) error {
	// Production backend-zero iterates 4 device keys; the lightweight rebuild
	// does not maintain per-device pointers, so this is a no-op for now.
	_ = ctx
	_ = uid
	return nil
}

func (s *SessionStore) revoke(ctx context.Context, access, refresh string) error {
	if s.rdb == nil {
		return nil
	}
	_, _ = s.rdb.Del(sessKeyPrefix + access)
	if refresh != "" {
		_, _ = s.rdb.Del(refreshKeyPrefix + refresh)
	}
	return nil
}

func (s *SessionStore) persist(ctx context.Context, sess *Session) error {
	if s.rdb == nil {
		return errors.New("auth: redis unavailable")
	}
	body, err := json.Marshal(sess)
	if err != nil {
		return err
	}
	// Compute TTL: use refresh expiry for user, access expiry for guest.
	ttl := int(s.userRefreshTTL.Seconds())
	if sess.IsGuest {
		ttl = int(s.guestAccessTTL.Seconds())
	}
	if ttl <= 0 {
		ttl = int(DefaultUserRefreshTTL.Seconds())
	}
	if err := s.rdb.Setex(sessKeyPrefix+sess.AccessToken, string(body), ttl); err != nil {
		return fmt.Errorf("auth: set sess: %w", err)
	}
	if !sess.IsGuest && sess.RefreshToken != "" {
		if err := s.rdb.Setex(refreshKeyPrefix+sess.RefreshToken, sess.AccessToken, ttl); err != nil {
			return fmt.Errorf("auth: set refresh: %w", err)
		}
	}
	return nil
}

// ----- helpers -----

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// GuestNickName returns a 6-uppercase-hex-char name like "A1F4C9".
func GuestNickName() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return upperHex(b)
}

func upperHex(b []byte) string {
	out := make([]byte, len(b)*2)
	hex.Encode(out, b)
	// uppercase
	for i := range out {
		if out[i] >= 'a' && out[i] <= 'f' {
			out[i] -= 32
		}
	}
	return string(out)
}

func platToDevice(plat int) string {
	switch plat {
	case 1:
		return "android"
	case 2:
		return "ios"
	case 3:
		return "pc"
	case 4:
		return "h5"
	}
	return "h5"
}
