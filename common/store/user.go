package store

// MySQL-backed user store with zbyy-compatible password encryption.
//
// The zbyy client encrypts the password BEFORE sending it to the server:
//   md5Pwd(password, pwdType):
//     pwdType 2 => md5(password)
//     pwdType 1 => md5( md5(password.toLowerCase()) + "&%*$8@!!%" )
//
// Therefore the server stores the client-sent encrypted string as-is and
// compares it directly on login. The server NEVER sees the raw password.
//
// For convenience, if the API receives a raw password (PwdTypeRaw flag),
// the server can compute the hash via auth.Md5Pwd — but the normal client
// flow sends the pre-encrypted value.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"apipro/common/auth"
	"apipro/common/model"
	"apipro/pkg/fixture"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

var (
	ErrUserExists      = errors.New("user already exists")
	ErrUserNotFound    = errors.New("user not found")
	ErrInvalidPassword = errors.New("invalid password")
)

type UserStore struct {
	users *model.UserModel
	rdb   *redis.Redis // for optional session caching
}

func NewUserStore(users *model.UserModel, rdb *redis.Redis) *UserStore {
	return &UserStore{users: users, rdb: rdb}
}

// Register creates a new user.
//   loginName  — unique login name
//   cc         — country code e.g. "+86"
//   phone      — phone number
//   password   — the CLIENT-ENCRYPTED md5 string (zbyy md5Pwd)
//   pwdType    — 1 or 2 (matches zbyy client)
//   nickName   — display name (optional, defaults to loginName)
//
// If the caller passes a raw password instead, set rawPw=true and the server
// will compute auth.Md5Pwd(password, pwdType) before storing.
func (s *UserStore) Register(ctx context.Context, loginName, cc, phone, password string, pwdType int32, nickName string, rawPw bool) (fixture.UserRecord, error) {
	loginName = strings.TrimSpace(loginName)
	phone = strings.TrimSpace(phone)
	cc = strings.TrimSpace(cc)
	nickName = strings.TrimSpace(nickName)
	if nickName == "" {
		nickName = loginName
	}
	if loginName == "" || phone == "" || cc == "" {
		return fixture.UserRecord{}, errors.New("missing fields")
	}
	if password == "" {
		return fixture.UserRecord{}, errors.New("missing password")
	}

	// Compute the stored hash.
	var storedPwd string
	if rawPw {
		storedPwd = auth.Md5Pwd(password, int(pwdType))
	} else {
		// Client already encrypted — store as-is.
		storedPwd = password
	}

	uid := fixture.GenUid("U")
	u := &model.User{
		Uid: uid, LoginName: loginName, NickName: nickName,
		Phone: phone, CountryCode: cc, Password: storedPwd, PwdType: pwdType,
		Grow: 0, Score: 0, Level: 1,
		Avatar: "https://cdn.zbyy.example/avatar/default.png",
		IsUser: 1,
	}
	if err := s.users.Insert(ctx, u); err != nil {
		if errors.Is(err, model.ErrDuplicate) {
			return fixture.UserRecord{}, ErrUserExists
		}
		return fixture.UserRecord{}, err
	}
	return toRecord(u), nil
}

// Login validates credentials.
//   password — the CLIENT-ENCRYPTED md5 string (zbyy md5Pwd)
// If rawPw=true, the server computes auth.Md5Pwd(password, pwdType) first.
func (s *UserStore) Login(ctx context.Context, cc, phone, password string, pwdType int32, rawPw bool) (fixture.UserRecord, error) {
	u, err := s.users.FindByPhone(ctx, cc, phone)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return fixture.UserRecord{}, ErrUserNotFound
		}
		return fixture.UserRecord{}, err
	}

	var candidate string
	if rawPw {
		candidate = auth.Md5Pwd(password, int(pwdType))
	} else {
		candidate = password
	}

	if !auth.Verify(candidate, u.Password) {
		return fixture.UserRecord{}, ErrInvalidPassword
	}
	return toRecord(u), nil
}

// GetByUid fetches a user by uid.
func (s *UserStore) GetByUid(ctx context.Context, uid string) (fixture.UserRecord, error) {
	u, err := s.users.FindByUid(ctx, uid)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return fixture.UserRecord{}, ErrUserNotFound
		}
		return fixture.UserRecord{}, err
	}
	return toRecord(u), nil
}

// CreateGuest creates a transient guest user (persisted, is_user=0).
func (s *UserStore) CreateGuest(ctx context.Context) (fixture.UserRecord, error) {
	uid := fixture.GenUid("G")
	suffix := uid[len(uid)-4:]
	u := &model.User{
		Uid: uid, LoginName: "", NickName: "游客" + suffix,
		Phone: "", CountryCode: "", Password: "", PwdType: 1,
		Grow: 0, Score: 0, Level: 1,
		Avatar: "https://cdn.zbyy.example/avatar/guest.png",
		IsUser: 0,
	}
	if err := s.users.Insert(ctx, u); err != nil {
		// Guests have no unique phone/login_name; duplicate-key shouldn't trigger,
		// but if it does (empty phone clash), fall back to a random suffix.
		u.LoginName = "guest_" + suffix
		u.Phone = ""
		_ = s.users.Insert(ctx, u)
	}
	return toRecord(u), nil
}

func toRecord(u *model.User) fixture.UserRecord {
	return fixture.UserRecord{
		Uid: u.Uid, LoginName: u.LoginName, NickName: u.NickName,
		Phone: u.Phone, CountryCode: u.CountryCode, Password: "",
		Grow: u.Grow, Score: u.Score, Level: u.Level,
		Avatar: u.Avatar, IsUser: u.IsUser, CreatedAt: u.CreatedAt,
	}
}

// DebugString for logging
func (s *UserStore) DebugString(rec fixture.UserRecord) string {
	return fmt.Sprintf("uid=%s nick=%s isUser=%d", rec.Uid, rec.NickName, rec.IsUser)
}
