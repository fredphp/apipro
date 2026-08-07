package model

// User model — MySQL/SQLite CRUD for the `users` table.
// Passwords are stored as the client-sent md5 hash (zbyy algorithm).
// See common/auth/pwd.go for the encryption details.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type User struct {
	Uid         string
	LoginName   string
	NickName    string
	Phone       string
	CountryCode string
	Password    string `json:"-"` // never serialized
	PwdType     int32
	Grow        int64
	Score       int64
	Level       int32
	Avatar      string
	IsUser      int32
	CreatedAt   int64
	UpdatedAt   int64
}

var ErrNotFound = errors.New("record not found")
var ErrDuplicate = errors.New("duplicate record")

type UserModel struct {
	db *sql.DB
}

func NewUserModel(db *sql.DB) *UserModel { return &UserModel{db: db} }

const userCols = `uid, login_name, nick_name, phone, country_code, password, pwd_type, grow, score, level, avatar, is_user, created_at, updated_at`

func (m *UserModel) Insert(ctx context.Context, u *User) error {
	now := time.Now().Unix()
	if u.CreatedAt == 0 {
		u.CreatedAt = now
	}
	u.UpdatedAt = now
	_, err := m.db.ExecContext(ctx, `INSERT INTO users
		(uid, login_name, nick_name, phone, country_code, password, pwd_type, grow, score, level, avatar, is_user, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		u.Uid, u.LoginName, u.NickName, u.Phone, u.CountryCode, u.Password, u.PwdType,
		u.Grow, u.Score, u.Level, u.Avatar, u.IsUser, u.CreatedAt, u.UpdatedAt,
	)
	if err != nil {
		if isDup(err) {
			return ErrDuplicate
		}
		return err
	}
	return nil
}

func (m *UserModel) FindByUid(ctx context.Context, uid string) (*User, error) {
	row := m.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE uid=?`, uid)
	return scanUser(row)
}

func (m *UserModel) FindByPhone(ctx context.Context, cc, phone string) (*User, error) {
	row := m.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE country_code=? AND phone=?`, cc, phone)
	return scanUser(row)
}

func (m *UserModel) FindByLoginName(ctx context.Context, loginName string) (*User, error) {
	row := m.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE login_name=?`, loginName)
	return scanUser(row)
}

func scanUser(row *sql.Row) (*User, error) {
	var u User
	err := row.Scan(&u.Uid, &u.LoginName, &u.NickName, &u.Phone, &u.CountryCode,
		&u.Password, &u.PwdType, &u.Grow, &u.Score, &u.Level, &u.Avatar,
		&u.IsUser, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func isDup(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return contains(s, "Duplicate entry") || contains(s, "UNIQUE constraint failed")
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// DebugString for logging (no password).
func (u *User) DebugString() string {
	return fmt.Sprintf("uid=%s nick=%s isUser=%d", u.Uid, u.NickName, u.IsUser)
}
