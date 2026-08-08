package svc

// SMS code store — Redis-backed, with dev bypass.
//
// Key: yuyan:sms:<countryCode>:<phone>:<smsType>
// TTL: 5 minutes
// Verify is one-shot (deletes on success).
//
// Dev bypass: when Mode=dev AND SmsDevBypassCode is non-empty, that code is
// always accepted (without consuming the stored code). Production NEVER honors it.

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

const (
	smsKeyPrefix = "yuyan:sms:"
	smsTTL       = 5 * time.Minute
)

type SmsStore struct {
	rdb       *redis.Redis
	mode      string // "dev" or "prod"
	bypass    string // dev bypass code (only honored when mode=dev)
}

func NewSmsStore(rdb *redis.Redis, mode, bypass string) *SmsStore {
	return &SmsStore{rdb: rdb, mode: strings.ToLower(strings.TrimSpace(mode)), bypass: strings.TrimSpace(bypass)}
}

// Issue generates a 4-digit code, stores it in Redis (5min TTL), returns it.
// In dev mode, the dev bypass code is NOT stored — the caller can still use
// the bypass to verify.
func (s *SmsStore) Issue(ctx context.Context, cc, phone string, smsType int) (string, error) {
	code := randomDigits(4)
	key := s.key(cc, phone, smsType)
	if err := s.rdb.Setex(key, code, int(smsTTL.Seconds())); err != nil {
		return "", fmt.Errorf("sms: set: %w", err)
	}
	return code, nil
}

// Verify checks the code. One-shot (deletes on success). Returns true if:
//   - stored code matches the input, OR
//   - dev mode AND input equals the bypass code (without consuming).
func (s *SmsStore) Verify(ctx context.Context, cc, phone string, smsType int, input string) bool {
	if input == "" {
		return false
	}
	// Dev bypass
	if s.mode == "dev" && s.bypass != "" && input == s.bypass {
		return true
	}
	if s.rdb == nil {
		return false
	}
	key := s.key(cc, phone, smsType)
	stored, err := s.rdb.Get(key)
	if err != nil || stored == "" {
		return false
	}
	if stored != input {
		return false
	}
	_, _ = s.rdb.Del(key)
	return true
}

func (s *SmsStore) key(cc, phone string, smsType int) string {
	cc = strings.TrimPrefix(strings.TrimSpace(cc), "+")
	phone = strings.TrimSpace(phone)
	return fmt.Sprintf("%s%s:%s:%d", smsKeyPrefix, cc, phone, smsType)
}

func randomDigits(n int) string {
	const digits = "0123456789"
	out := make([]byte, n)
	for i := range out {
		b := make([]byte, 1)
		_, _ = rand.Read(b)
		out[i] = digits[int(b[0])%len(digits)]
	}
	return string(out)
}
