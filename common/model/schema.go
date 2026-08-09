// Package model — schema name resolution for multi-database MySQL layouts.
//
// In production, apipro mirrors the backend-zero data layout: tables are
// spread across multiple MySQL databases (schemas) using a shared prefix.
// The default prefix is "zb_" (matching the user's local environment), so
// e.g. the user table lives at `zb_user.user`, the live_room table at
// `zb_live.live_room`, and so on.
//
// All SQL in this package routes every bare table name through Tbl(), which
// returns the fully-qualified `<schema>.<table>` form in MySQL mode, or the
// bare `<table>` form when SQLite is in use (SQLite has no cross-database
// namespaces in the MySQL sense — a single file holds all tables).
//
// The schema-name → table mapping is fixed (matches backend-zero); the prefix
// itself is configurable via SetSchemaPrefix() so a different deployment can
// rename the whole set (e.g. "haima_" for the upstream backend-zero).
package model

import "strings"

// Default schema prefix used in production. Override with SetSchemaPrefix().
const defaultSchemaPrefix = "zb_"

// schemaPrefix is the active prefix (e.g. "zb_", "haima_"). Empty is allowed
// and means "no prefix — use the bare schema short-name" (NOT recommended;
// prefer SetNoSchemaPrefix() for SQLite which disables qualification entirely).
var schemaPrefix = defaultSchemaPrefix

// useSchemaPrefix controls whether Tbl() emits the `<schema>.<table>` form.
// False for SQLite (single-file database); true for MySQL.
var useSchemaPrefix = true

// tableSchema maps each table name to its schema short-name (without prefix).
// Mirrors backend-zero's haima_user / haima_live / haima_chat / haima_gift /
// haima_admin / haima_sys boundaries — except prefixed "zb_" instead of
// "haima_" to match the user's actual local MySQL layout.
var tableSchema = map[string]string{
	// zb_user — user identity, growth, scoring, gift-rank aggregates
	"user":               "user",
	"user_grow":          "user",
	"room_gift_rank":     "user",
	"uid_pool":           "user",
	"account":            "user",
	"attention_compere":  "user",
	"login_log":          "user",
	"reg_info":           "user",

	// zb_live — live type catalog, rooms, anchors, match schedule
	"live_type":             "live",
	"live_room":             "live",
	"anchors":               "live",
	"match_schedule":        "live",
	"match_schedule_room":   "live",
	"live_hot_recommend":    "live",
	"high_light":            "live",
	"live_stat":             "live",

	// zb_chat — live-room chat history
	"chat_room_message": "chat",

	// zb_gift — gift catalog
	"gift": "gift",

	// zb_admin — admin users / ops
	"admin_user": "admin",

	// zb_sys — system messages, ads, articles
	"advertising":          "sys",
	"article":              "sys",
	"system_chat_message":  "sys",
	"system_notice_message": "sys",

	// Legacy tables (kept for backwards-compat; not used by the new protocol).
	"rooms":       "live",
	"room_ranks":  "user",
}

// SetSchemaPrefix overrides the schema name prefix (e.g. "zb_", "haima_").
// Pass "" to use bare short-names ("user", "live", ...) — only meaningful
// in MySQL when the schemas are renamed without a prefix.
func SetSchemaPrefix(p string) {
	schemaPrefix = strings.TrimSpace(p)
}

// SetNoSchemaPrefix disables schema qualification entirely so Tbl() returns
// the bare table name. Use this when running on SQLite (single-file DB).
func SetNoSchemaPrefix() {
	useSchemaPrefix = false
}

// SchemaForTable returns the fully-qualified schema name for a table, e.g.
// Tbl("user") → "zb_user". Returns "" if the table is unknown or schema
// qualification is disabled.
func SchemaForTable(table string) string {
	if !useSchemaPrefix {
		return ""
	}
	short := tableSchema[table]
	if short == "" {
		return ""
	}
	return schemaPrefix + short
}

// Tbl returns the fully-qualified table name for use in SQL.
//
// MySQL mode:  Tbl("user")         → "zb_user.user"
//              Tbl("live_room")    → "zb_live.live_room"
// SQLite mode: Tbl("user")         → "user"
//              Tbl("live_room")    → "live_room"
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
