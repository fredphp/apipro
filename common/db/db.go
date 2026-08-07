package db

// Database connection layer.
// Supports MySQL (production) and SQLite (dev/self-check) via database/sql.
// The driver is chosen by the DBDriver config field.

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"   // MySQL driver
	_ "modernc.org/sqlite"               // Pure-Go SQLite driver (no CGO)
)

// New opens a *sql.DB for the given driver ("mysql" or "sqlite") and DSN.
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

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("db: ping %s: %w", driver, err)
	}
	return db, nil
}

// MustNew panics on error (for ServiceContext init).
func MustNew(driver, dsn string) *sql.DB {
	d, err := New(driver, dsn)
	if err != nil {
		panic(err)
	}
	return d
}
