// Package model — schema name resolution for multi-database MySQL layouts.
//
// In production, apipro mirrors the backend-zero data layout: tables are
// spread across multiple MySQL databases (schemas) using a shared prefix.
// The default prefix is "zb_" (matching the user's local environment), so
// e.g. the user table lives at `zb_user.user`, the live_room table at
// `zb_live.live_room`, and so on.
//
// A second prefix ("eim_" by default) covers the IM-related schemas that
// live alongside the zb_* set: eim_user / eim_friend / eim_group / eim_admin.
//
// All SQL in this package routes every bare table name through Tbl(), which
// returns the fully-qualified `<schema>.<table>` form in MySQL mode, or the
// bare `<table>` form when SQLite is in use (SQLite has no cross-database
// namespaces in the MySQL sense — a single file holds all tables).
//
// The schema-name → table mapping is fixed (matches backend-zero); the prefix
// itself is configurable via SetSchemaPrefix() / SetEimSchemaPrefix() so a
// different deployment can rename the whole set (e.g. "haima_" for the
// upstream backend-zero).
package model

import "strings"

// Default schema prefixes used in production. Override with SetSchemaPrefix()
// / SetEimSchemaPrefix().
const (
	defaultSchemaPrefix    = "zb_"
	defaultEimSchemaPrefix = "eim_"
)

// schemaPrefix is the active prefix for the main app schemas (e.g. "zb_",
// "haima_"). Empty is allowed and means "no prefix — use the bare schema
// short-name" (NOT recommended; prefer SetNoSchemaPrefix() for SQLite which
// disables qualification entirely).
var schemaPrefix = defaultSchemaPrefix

// eimSchemaPrefix is the active prefix for IM-related schemas (e.g. "eim_").
var eimSchemaPrefix = defaultEimSchemaPrefix

// useSchemaPrefix controls whether Tbl() emits the `<schema>.<table>` form.
// False for SQLite (single-file database); true for MySQL.
var useSchemaPrefix = true

// tableSchema maps each table name to its schema short-name (without prefix).
// Mirrors backend-zero's haima_user / haima_live / haima_chat / haima_gift /
// haima_admin / haima_sys boundaries — except prefixed "zb_" instead of
// "haima_" to match the user's actual local MySQL layout.
//
// The eim_* schemas (eim_user / eim_friend / eim_group / eim_admin) are
// tagged with the special sentinel "eim:<short>" so Tbl() knows to apply the
// EIM prefix instead of the main app prefix.
var tableSchema = map[string]string{
	// zb_user — user identity, growth, scoring, gift-rank aggregates
	"user":              "user",
	"user_grow":         "user",
	"room_gift_rank":    "user",
	"uid_pool":          "user",
	"account":           "user",
	"attention_compere": "user",
	"login_log":         "user",
	"reg_info":          "user",

	// zb_live — live type catalog, rooms, anchors, match schedule
	"live_type":           "live",
	"live_room":           "live",
	"anchors":             "live",
	"match_schedule":      "live",
	"match_schedule_room": "live",
	"live_hot_recommend":  "live",
	"high_light":          "live",
	"live_stat":           "live",

	// zb_chat — live-room chat history
	"chat_room_message": "chat",

	// zb_gift — gift catalog
	"gift": "gift",

	// zb_admin — admin users / ops
	"admin_user": "admin",

	// zb_sys — system messages, ads, articles
	"advertising":           "sys",
	"article":               "sys",
	"system_chat_message":   "sys",
	"system_notice_message": "sys",

	// zb_basketball — basketball sport data (teams, leagues, standings)
	// Reserved for future basketball-specific tables; currently no tables
	// are mapped here because match_schedule already covers game scheduling
	// in zb_live. These schemas exist in the user's MySQL server and are
	// declared so the schema-prefix machinery knows about them.
	"basketball_team":    "basketball",
	"basketball_league":  "basketball",
	"basketball_stand":   "basketball",
	"basketball_player":  "basketball",

	// zb_football — football sport data (teams, leagues, standings)
	"football_team":    "football",
	"football_league":  "football",
	"football_stand":   "football",
	"football_player":  "football",

	// eim_user — IM user profiles / online state (uses EIM prefix)
	"eim_user":          "eim:user",
	"eim_user_online":   "eim:user",

	// eim_friend — IM friend relationships
	"eim_friend":        "eim:friend",
	"eim_friend_group":  "eim:friend",
	"eim_friend_apply":  "eim:friend",

	// eim_group — IM group chat
	"eim_group":         "eim:group",
	"eim_group_member":  "eim:group",
	"eim_group_msg":     "eim:group",

	// eim_admin — IM admin / config
	"eim_admin":         "eim:admin",
	"eim_app_config":    "eim:admin",

	// Legacy tables (kept for backwards-compat; not used by the new protocol).
	"rooms":      "live",
	"room_ranks": "user",
}

// SetSchemaPrefix overrides the main app schema-name prefix (e.g. "zb_",
// "haima_"). Pass "" to use bare short-names ("user", "live", ...) — only
// meaningful in MySQL when the schemas are renamed without a prefix.
func SetSchemaPrefix(p string) {
	schemaPrefix = strings.TrimSpace(p)
}

// SetEimSchemaPrefix overrides the IM schema-name prefix (e.g. "eim_").
// Pass "" to use bare short-names ("user", "friend", ...).
func SetEimSchemaPrefix(p string) {
	eimSchemaPrefix = strings.TrimSpace(p)
}

// SetNoSchemaPrefix disables schema qualification entirely so Tbl() returns
// the bare table name. Use this when running on SQLite (single-file DB).
func SetNoSchemaPrefix() {
	useSchemaPrefix = false
}

// SchemaForTable returns the fully-qualified schema name for a table, e.g.
// Tbl("user") → "zb_user", Tbl("eim_user") → "eim_user". Returns "" if the
// table is unknown or schema qualification is disabled.
func SchemaForTable(table string) string {
	if !useSchemaPrefix {
		return ""
	}
	short := tableSchema[table]
	if short == "" {
		return ""
	}
	return resolveSchemaName(short)
}

// resolveSchemaName converts a schema short-name to its fully-qualified form,
// applying the appropriate prefix (main app vs EIM).
func resolveSchemaName(short string) string {
	if len(short) > 4 && short[:4] == "eim:" {
		return eimSchemaPrefix + short[4:]
	}
	return schemaPrefix + short
}

// isEimSchema reports whether a schema short-name is an EIM schema.
func isEimSchema(short string) bool {
	return len(short) > 4 && short[:4] == "eim:"
}

// Tbl returns the fully-qualified table name for use in SQL.
//
// MySQL mode:  Tbl("user")         → "zb_user.user"
//              Tbl("live_room")    → "zb_live.live_room"
//              Tbl("eim_user")     → "eim_user.eim_user"
// SQLite mode: Tbl("user")         → "user"
//              Tbl("live_room")    → "live_room"
//              Tbl("eim_user")     → "eim_user"
//
// Unknown tables fall back to the bare name (so ad-hoc queries aren't broken).
func Tbl(table string) string {
	if !useSchemaPrefix {
		return table
	}
	sch := SchemaForTable(table)
	if sch == "" {
		return table
	}
	return sch + "." + table
}

// SchemaShort returns the schema short-name (without prefix) for a table,
// e.g. SchemaShort("user") → "user", SchemaShort("eim_user") → "eim:user".
// Returns "" if the table is unknown or schema qualification is disabled.
// Used by MultiDB.ForSchema() to pick the right connection pool.
func SchemaShort(table string) string {
	if !useSchemaPrefix {
		return ""
	}
	return tableSchema[table]
}

// SchemaUser returns the qualified user-schema name (e.g. "zb_user"), or ""
// in SQLite mode. Convenience for hand-written SQL that needs the schema
// itself rather than a specific table.
func SchemaUser() string { return SchemaForTable("user") }

// SchemaLive returns the qualified live-schema name (e.g. "zb_live").
func SchemaLive() string { return SchemaForTable("live_room") }

// SchemaChat returns the qualified chat-schema name (e.g. "zb_chat").
func SchemaChat() string { return SchemaForTable("chat_room_message") }

// SchemaGift returns the qualified gift-schema name (e.g. "zb_gift").
func SchemaGift() string { return SchemaForTable("gift") }

// SchemaAdmin returns the qualified admin-schema name (e.g. "zb_admin").
func SchemaAdmin() string { return SchemaForTable("admin_user") }

// SchemaSys returns the qualified sys-schema name (e.g. "zb_sys").
func SchemaSys() string { return SchemaForTable("article") }

// SchemaBasketball returns the qualified basketball-schema name (e.g. "zb_basketball").
func SchemaBasketball() string { return SchemaForTable("basketball_team") }

// SchemaFootball returns the qualified football-schema name (e.g. "zb_football").
func SchemaFootball() string { return SchemaForTable("football_team") }

// SchemaEimUser returns the qualified EIM user-schema name (e.g. "eim_user").
func SchemaEimUser() string { return SchemaForTable("eim_user") }

// SchemaEimFriend returns the qualified EIM friend-schema name (e.g. "eim_friend").
func SchemaEimFriend() string { return SchemaForTable("eim_friend") }

// SchemaEimGroup returns the qualified EIM group-schema name (e.g. "eim_group").
func SchemaEimGroup() string { return SchemaForTable("eim_group") }

// SchemaEimAdmin returns the qualified EIM admin-schema name (e.g. "eim_admin").
func SchemaEimAdmin() string { return SchemaForTable("eim_admin") }
