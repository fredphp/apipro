package db

// MultiDB — multi-database connection manager.
//
// Supports two modes:
//
//  1. Shared-pool mode (DEFAULT, recommended): a single *sql.DB backed by a
//     DSN with an EMPTY database path (e.g. "user:pass@tcp(host:port)/?...").
//     All schemas (zb_user, zb_live, zb_chat, eim_user, ...) are accessible
//     via fully-qualified table names (zb_user.user, zb_live.live_room, ...)
//     and cross-schema JOINs work transparently. This is how the production
//     MySQL layout is meant to be consumed.
//
//  2. Per-schema-pool mode (OPTIONAL): each schema group gets its own *sql.DB
//     pinned to a specific database (e.g. "user:pass@tcp(host:port)/zb_user?...").
//     Use this when different schemas have different credentials or when you
//     want connection isolation per schema. Cross-schema JOINs will NOT work
//     in this mode unless the MySQL user has grants across all schemas AND
//     you also configure a shared pool (via DataSource) for models that need
//     cross-schema access.
//
// Resolution rules:
//   - ForSchema(shortName) returns the per-schema pool if configured, else
//     falls back to the shared pool. Use this for single-schema models.
//   - Shared() always returns the shared pool. Use this for models that do
//     cross-schema JOINs (e.g. match_schedule JOIN user JOIN live_room).
//
// When SQLite is in use, the shared pool is the single *sql.DB and
// ForSchema() always returns it (SQLite has no cross-database namespaces).

import (
        "database/sql"
        "fmt"
        "strings"

        "github.com/zeromicro/go-zero/core/logx"
)

// MultiDB holds the shared connection pool plus optional per-schema pools.
type MultiDB struct {
        shared *sql.DB            // always non-nil; used for cross-schema JOINs + fallback
        pools  map[string]*sql.DB // per-schema pools (keyed by schema short-name like "user", "live", "eim:user")
        driver string
}

// SchemaShortener is implemented by model.SchemaShort. It maps a table name
// to its schema short-name (e.g. "user" → "user", "eim_user" → "eim:user").
// Injected as a parameter to avoid an import cycle (model → db → model).
type SchemaShortener func(table string) string

// NewMultiDB builds a MultiDB from the config.
//
// Parameters:
//   - driver: "mysql" or "sqlite"
//   - sharedDSN: the shared DSN (empty path for MySQL cross-schema mode).
//     Required for MySQL; for SQLite this is the single-file DSN.
//   - databases: optional per-schema DSN map (keyed by schema short-name).
//     Keys can be bare short-names ("user", "live", "chat") or EIM-tagged
//     ("eim:user", "eim:friend"). For convenience, "eim_user" / "eim_friend"
//     are also accepted as aliases for "eim:user" / "eim:friend".
//
// For SQLite, per-schema pools are supported (each is a separate SQLite
// connection, typically a separate file). This is mainly useful for testing;
// in production SQLite deployments, the Databases map is usually empty.
func NewMultiDB(driver, sharedDSN string, databases map[string]string) (*MultiDB, error) {
        if driver == "" {
                driver = "mysql"
        }

        mdb := &MultiDB{
                driver: driver,
                pools:  make(map[string]*sql.DB),
        }

        // Open the shared pool (required for both MySQL and SQLite).
        if sharedDSN == "" && len(databases) == 0 {
                return nil, fmt.Errorf("multidb: DataSource (shared DSN) is required; set it to an empty-path DSN for MySQL cross-schema mode, e.g. root:pass@tcp(host:port)/?charset=..., or a file path for SQLite, e.g. ./data/apipro.db")
        }
        if sharedDSN == "" && len(databases) > 0 {
                // No shared DSN but Databases map is set — use the first Databases
                // entry as the shared pool (cross-schema JOINs will fail in this case).
                for schemaKey, dsn := range databases {
                        sharedDSN = strings.TrimSpace(dsn)
                        logx.Infof("multidb: no shared DataSource; using first Databases entry %q as shared pool", schemaKey)
                        break
                }
        }

        shared, err := New(driver, sharedDSN)
        if err != nil {
                return nil, fmt.Errorf("multidb: shared open: %w", err)
        }
        mdb.shared = shared

        // Open per-schema pools when configured (works for both MySQL and SQLite).
        for schemaKey, dsn := range databases {
                schemaKey = strings.TrimSpace(schemaKey)
                dsn = strings.TrimSpace(dsn)
                if schemaKey == "" || dsn == "" {
                        continue
                }
                // Normalize "eim_user" / "eim_friend" → "eim:user" / "eim:friend"
                normalized := normalizeSchemaKey(schemaKey)
                pool, err := New(driver, dsn)
                if err != nil {
                        // Don't fail hard — log and continue. The shared pool will cover
                        // this schema as a fallback.
                        logx.Errorf("multidb: open per-schema pool %q: %v (falling back to shared pool)", schemaKey, err)
                        continue
                }
                mdb.pools[normalized] = pool
                logx.Infof("multidb: per-schema pool registered for %q (normalized: %q)", schemaKey, normalized)
        }

        if len(mdb.pools) > 0 {
                logx.Infof("multidb: %d per-schema pool(s) active + 1 shared pool", len(mdb.pools))
        } else {
                logx.Infof("multidb: shared-pool mode (cross-schema queries enabled)")
        }

        return mdb, nil
}

// MustNewMultiDB panics on error (for ServiceContext init).
func MustNewMultiDB(driver, sharedDSN string, databases map[string]string) *MultiDB {
        mdb, err := NewMultiDB(driver, sharedDSN, databases)
        if err != nil {
                panic(err)
        }
        return mdb
}

// Shared returns the shared connection pool. Always non-nil.
// Use this for models that do cross-schema JOINs.
func (m *MultiDB) Shared() *sql.DB {
        if m == nil {
                return nil
        }
        return m.shared
}

// ForSchema returns the connection pool for a specific schema short-name.
// If a per-schema pool is configured for that schema, it's returned; otherwise
// the shared pool is returned as a fallback.
//
// schemaShort follows the model.SchemaShort() convention:
//   - "user", "live", "chat", "gift", "admin", "sys", "basketball", "football"
//   - "eim:user", "eim:friend", "eim:group", "eim:admin"
//
// For convenience, bare EIM names ("eim_user", "eim_friend", ...) are also
// accepted and normalized internally.
func (m *MultiDB) ForSchema(schemaShort string) *sql.DB {
        if m == nil {
                return nil
        }
        normalized := normalizeSchemaKey(schemaShort)
        if pool, ok := m.pools[normalized]; ok {
                return pool
        }
        return m.shared
}

// ForTable returns the connection pool appropriate for a given table name.
// Uses the schemaShorter function (typically model.SchemaShort) to resolve
// the table → schema mapping, then delegates to ForSchema().
//
// If the table is unknown (schemaShorter returns ""), the shared pool is
// returned (cross-schema fallback).
func (m *MultiDB) ForTable(tableName string, schemaShorter SchemaShortener) *sql.DB {
        if m == nil || schemaShorter == nil {
                return m.shared
        }
        short := schemaShorter(tableName)
        if short == "" {
                return m.shared
        }
        return m.ForSchema(short)
}

// Close closes all connection pools (shared + per-schema).
func (m *MultiDB) Close() {
        if m == nil {
                return
        }
        if m.shared != nil {
                _ = m.shared.Close()
        }
        for k, p := range m.pools {
                _ = p.Close()
                delete(m.pools, k)
        }
}

// HasPerSchemaPools reports whether any per-schema pools are configured.
// When false, ForSchema() always returns the shared pool.
func (m *MultiDB) HasPerSchemaPools() bool {
        return m != nil && len(m.pools) > 0
}

// normalizeSchemaKey converts a user-facing schema key into the canonical
// internal form used by the pools map.
//
//   - "user"         → "user"
//   - "live"         → "live"
//   - "eim_user"     → "eim:user"
//   - "eim:friend"   → "eim:friend"  (already canonical)
//   - "eimfriend"    → "eim:friend"  (heuristic: if it starts with "eim" and
//                                     isn't "eim:" already, insert the colon)
func normalizeSchemaKey(key string) string {
        key = strings.TrimSpace(key)
        if key == "" {
                return ""
        }
        // Already in canonical "eim:xxx" form?
        if strings.HasPrefix(key, "eim:") {
                return key
        }
        // Bare "eim_xxx" → "eim:xxx"
        if strings.HasPrefix(key, "eim_") {
                return "eim:" + key[4:]
        }
        // Bare "eimxxx" (no underscore) → "eim:xxx" — but only when the remainder
        // is non-empty and the key doesn't look like a regular main-app schema.
        if strings.HasPrefix(key, "eim") && len(key) > 3 && key != "eim" {
                return "eim:" + key[3:]
        }
        return key
}
