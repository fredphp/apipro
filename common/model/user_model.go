package model

// User model — MySQL/SQLite CRUD for the `user` table.
// Passwords are stored as md5(client_md5 + salt) — pwd_type=2 only.

import (
        "context"
        "database/sql"
        "errors"
        "fmt"
        "time"
)

type User struct {
        UID         int64
        LoginName   string
        NickName    string
        Phone       string
        CountryCode string // normalized, no +, e.g. "86"
        Password    string `json:"-"` // md5(client_md5 + salt) lowercase hex
        Salt        string `json:"-"` // base64(32 random bytes), 44 chars
        PwdType     int32
        UserType    int32 // 1=audience 2=anchor 3=admin
        Score       int64
        Grow        int64
        Status      int32 // 1=normal, 2/3=banned
        Icon        string
        Gender      int32
        Birthday    sql.NullTime
        Plat        int32
        CreatedAt   int64
        UpdatedAt   int64
}

var (
        ErrNotFound = errors.New("record not found")
        ErrDuplicate = errors.New("duplicate record")
)

type UserModel struct {
        db *sql.DB
}

func NewUserModel(db *sql.DB) *UserModel { return &UserModel{db: db} }

const userCols = `uid, login_name, nick_name, phone, country_code, password, salt, pwd_type, user_type, score, grow, status, icon, gender, birthday, plat, created_at, updated_at`

func (m *UserModel) Insert(ctx context.Context, u *User) error {
        now := time.Now().Unix()
        if u.CreatedAt == 0 {
                u.CreatedAt = now
        }
        u.UpdatedAt = now
        var birthday interface{}
        if u.Birthday.Valid {
                birthday = u.Birthday.Time
        } else {
                birthday = nil
        }
        _, err := m.db.ExecContext(ctx, `INSERT INTO user
                (uid, login_name, nick_name, phone, country_code, password, salt, pwd_type, user_type, score, grow, status, icon, gender, birthday, plat, created_at, updated_at)
                VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
                u.UID, u.LoginName, u.NickName, u.Phone, u.CountryCode, u.Password, u.Salt, u.PwdType,
                u.UserType, u.Score, u.Grow, u.Status, u.Icon, u.Gender, birthday, u.Plat, u.CreatedAt, u.UpdatedAt,
        )
        if err != nil {
                if isDup(err) {
                        return ErrDuplicate
                }
                return err
        }
        return nil
}

func (m *UserModel) FindByUid(ctx context.Context, uid int64) (*User, error) {
        row := m.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM user WHERE uid=?`, uid)
        return scanUser(row)
}

func (m *UserModel) FindByPhone(ctx context.Context, cc, phone string) (*User, error) {
        row := m.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM user WHERE country_code=? AND phone=?`, cc, phone)
        return scanUser(row)
}

func (m *UserModel) FindByLoginName(ctx context.Context, loginName string) (*User, error) {
        row := m.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM user WHERE login_name=?`, loginName)
        return scanUser(row)
}

// NextUID reserves a new uid from a monotonically increasing counter.
// (backend-zero uses a uid_pool table; we simplify with MAX(uid)+1.)
//
// NOTE: AUDIT-008 — this is non-atomic and prone to race conditions on
// concurrent registration. Production code should use ServiceContext.AllocUID
// (Redis INCR) instead. This method is retained as the DB-only fallback.
func (m *UserModel) NextUID(ctx context.Context) (int64, error) {
        var maxUID int64
        _ = m.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(uid), 1000) FROM user`).Scan(&maxUID)
        // Start from 5000+ for audience users to avoid clashing with seeded anchors (1001-1003)
        if maxUID < 5000 {
                return 5001, nil
        }
        return maxUID + 1, nil
}

// MaxUID returns the largest uid in the user table (used to bootstrap the
// Redis UID counter). Returns 0 on empty table.
func (m *UserModel) MaxUID(ctx context.Context) (int64, error) {
        var maxUID sql.NullInt64
        err := m.db.QueryRowContext(ctx, `SELECT MAX(uid) FROM user`).Scan(&maxUID)
        if err != nil {
                if err == sql.ErrNoRows {
                        return 0, nil
                }
                return 0, err
        }
        return maxUID.Int64, nil
}

func scanUser(row *sql.Row) (*User, error) {
        var u User
        err := row.Scan(&u.UID, &u.LoginName, &u.NickName, &u.Phone, &u.CountryCode,
                &u.Password, &u.Salt, &u.PwdType, &u.UserType, &u.Score, &u.Grow, &u.Status,
                &u.Icon, &u.Gender, &u.Birthday, &u.Plat, &u.CreatedAt, &u.UpdatedAt)
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
        return fmt.Sprintf("uid=%d nick=%s type=%d status=%d", u.UID, u.NickName, u.UserType, u.Status)
}

// UserGrow represents a row in user_grow (level lookup).
type UserGrow struct {
        ID          int64
        Name        string
        MinGrow     int64
        Sort        int32
        Status      int32
        NextMinGrow int64 // computed via subquery; 0 if top level
}

// FindUserGrowForValue returns the grow level whose min_grow <= value, plus the
// next threshold (NextMinGrow). Returns a zero-value UserGrow if no rows.
func (m *UserModel) FindUserGrowForValue(ctx context.Context, value int64) (*UserGrow, error) {
        row := m.db.QueryRowContext(ctx, `
                SELECT g.id, g.name, g.min_grow, g.sort, g.status,
                       COALESCE((SELECT MIN(g2.min_grow) FROM user_grow g2 WHERE g2.min_grow > g.min_grow AND g2.status=1), 0)
                FROM user_grow g
                WHERE g.status=1 AND g.min_grow <= ?
                ORDER BY g.min_grow DESC LIMIT 1`, value)
        var g UserGrow
        if err := row.Scan(&g.ID, &g.Name, &g.MinGrow, &g.Sort, &g.Status, &g.NextMinGrow); err != nil {
                if err == sql.ErrNoRows {
                        return &UserGrow{ID: 0, Name: "LV.1", MinGrow: 0, NextMinGrow: 100}, nil
                }
                return nil, err
        }
        return &g, nil
}
