package db

// Database connection layer.
// Supports MySQL (production) and SQLite (dev/self-check) via database/sql.
// For SQLite, the schema is auto-created from deploy/schema.sqlite.sql on open.

import (
        "context"
        "database/sql"
        _ "embed"
        "fmt"
        "strings"
        "time"

        _ "github.com/go-sql-driver/mysql" // MySQL driver
        "github.com/zeromicro/go-zero/core/logx"
        _ "modernc.org/sqlite" // Pure-Go SQLite driver (no CGO)
)

//go:embed schema.sqlite.sql
var sqliteSchema string

// New opens a *sql.DB for the given driver ("mysql" or "sqlite") and DSN.
// For SQLite, it auto-applies the schema file.
//
// MySQL ping failure is NON-FATAL (audit-1B DZ-3 fix): the sandbox environment
// cannot reach the user's MySQL (3.1.198.77:3306) by design. The returned *sql.DB
// is still usable — queries will lazily attempt to connect and fail per-call,
// allowing the rest of the system (cache/Redis fallback) to keep serving.
func New(driver, dsn string) (*sql.DB, error) {
        if driver == "" {
                driver = "mysql"
        }
        if dsn == "" {
                return nil, fmt.Errorf("db: empty datasource for driver %s", driver)
        }

        var (
                db  *sql.DB
                err error
        )
        switch driver {
        case "mysql":
                db, err = sql.Open("mysql", dsn)
        case "sqlite", "sqlite3":
                db, err = sql.Open("sqlite", dsn)
        default:
                return nil, fmt.Errorf("db: unsupported driver %q", driver)
        }
        if err != nil {
                return nil, fmt.Errorf("db: open %s: %w", driver, err)
        }

        // Connection pool tuning.
        db.SetMaxOpenConns(50)
        db.SetMaxIdleConns(10)
        db.SetConnMaxLifetime(30 * time.Minute)
        db.SetConnMaxIdleTime(5 * time.Minute)

        // DZ-3 fix: bounded ping timeout; MySQL ping failure is non-fatal.
        pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer pingCancel()
        if err := db.PingContext(pingCtx); err != nil {
                if driver == "mysql" {
                        // Non-fatal: log and continue with lazy-connect semantics.
                        // The cache + Redis layer will keep serving until DB recovers.
                        logx.Errorf("db: ping %s failed (continuing with lazy connect...): %v", driver, err)
                } else {
                        return nil, fmt.Errorf("db: ping %s: %w", driver, err)
                }
        }

        // SQLite: auto-apply schema (idempotent — all CREATE TABLE IF NOT EXISTS).
        // database/sql's Exec only runs the FIRST statement when the driver
        // doesn't support multi-statement — split on `;` and exec each.
        if driver == "sqlite" || driver == "sqlite3" {
                for _, stmt := range splitSQLStatements(sqliteSchema) {
                        stmt = strings.TrimSpace(stmt)
                        if stmt == "" {
                                continue
                        }
                        if _, err := db.Exec(stmt); err != nil {
                                return nil, fmt.Errorf("db: apply sqlite schema stmt: %w\nstmt: %s", err, truncate(stmt, 200))
                        }
                }
                // Enable foreign keys + WAL for better concurrency.
                _, _ = db.Exec("PRAGMA journal_mode=WAL;")
                _, _ = db.Exec("PRAGMA foreign_keys=ON;")
                logx.Info("db: sqlite schema auto-applied")
        }
        return db, nil
}

// MustNew panics on error (for ServiceContext init).
// NOTE: For MySQL, ping failure is non-fatal (see New), so MustNew will NOT panic
// on MySQL connection issues — only on truly fatal errors (empty DSN, unsupported driver).
func MustNew(driver, dsn string) *sql.DB {
        d, err := New(driver, dsn)
        if err != nil {
                panic(err)
        }
        return d
}

// splitSQLStatements splits a multi-statement SQL script into individual
// statements on `;` boundaries (ignoring `;` inside string literals — a
// simple heuristic that's good enough for our schema file).
func splitSQLStatements(script string) []string {
        var out []string
        var buf strings.Builder
        inSingle := false
        inDouble := false
        for i := 0; i < len(script); i++ {
                c := script[i]
                switch {
                case c == '\'' && !inDouble:
                        inSingle = !inSingle
                        buf.WriteByte(c)
                case c == '"' && !inSingle:
                        inDouble = !inDouble
                        buf.WriteByte(c)
                case c == ';' && !inSingle && !inDouble:
                        stmt := strings.TrimSpace(buf.String())
                        if stmt != "" {
                                out = append(out, stmt)
                        }
                        buf.Reset()
                default:
                        buf.WriteByte(c)
                }
        }
        if buf.Len() > 0 {
                stmt := strings.TrimSpace(buf.String())
                if stmt != "" {
                        out = append(out, stmt)
                }
        }
        return out
}

func truncate(s string, n int) string {
        if len(s) <= n {
                return s
        }
        return s[:n] + "..."
}
