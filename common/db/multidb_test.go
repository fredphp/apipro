package db

import "testing"

// TestNormalizeSchemaKey verifies the schema key normalization logic that
// converts user-facing keys (e.g. "eim_user") to the canonical internal
// form (e.g. "eim:user") used by the pools map.
func TestNormalizeSchemaKey(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// Main app schemas (unchanged)
		{"user", "user"},
		{"live", "live"},
		{"chat", "chat"},
		{"gift", "gift"},
		{"admin", "admin"},
		{"sys", "sys"},
		{"basketball", "basketball"},
		{"football", "football"},

		// EIM schemas — various input forms
		{"eim_user", "eim:user"},
		{"eim_friend", "eim:friend"},
		{"eim_group", "eim:group"},
		{"eim_admin", "eim:admin"},

		// Already canonical (eim:xxx)
		{"eim:user", "eim:user"},
		{"eim:friend", "eim:friend"},

		// Edge cases
		{"", ""},
		{"  user  ", "user"},     // trimmed
		{"eim", "eim"},           // bare "eim" stays (no colon, no suffix)
		{"eim_", "eim:"},         // "eim_" with empty suffix
		{"User", "User"},         // case preserved (not lowercased)
	}
	for _, c := range cases {
		got := normalizeSchemaKey(c.input)
		if got != c.want {
			t.Errorf("normalizeSchemaKey(%q) = %q; want %q", c.input, got, c.want)
		}
	}
}

// TestMultiDBSharedPool verifies that in shared-pool mode (no Databases map),
// ForSchema always returns the shared pool.
func TestMultiDBSharedPool(t *testing.T) {
	// Use SQLite for testing (no external MySQL needed).
	mdb, err := NewMultiDB("sqlite", ":memory:", nil)
	if err != nil {
		t.Fatalf("NewMultiDB failed: %v", err)
	}
	defer mdb.Close()

	shared := mdb.Shared()
	if shared == nil {
		t.Fatal("Shared() returned nil")
	}

	// ForSchema should return the shared pool for ALL schemas (no per-schema
	// pools configured).
	for _, schema := range []string{"user", "live", "chat", "gift", "admin",
		"sys", "basketball", "football", "eim:user", "eim:friend",
		"eim:group", "eim:admin", "unknown"} {
		got := mdb.ForSchema(schema)
		if got != shared {
			t.Errorf("ForSchema(%q) returned a different pool than Shared() in shared-pool mode", schema)
		}
	}

	if mdb.HasPerSchemaPools() {
		t.Error("HasPerSchemaPools() = true; want false (no per-schema pools configured)")
	}
}

// TestMultiDBPerSchemaPool verifies that per-schema pools are used when
// configured, and ForSchema falls back to shared for unconfigured schemas.
func TestMultiDBPerSchemaPool(t *testing.T) {
	// Configure two per-schema pools (SQLite) + a shared pool.
	databases := map[string]string{
		"user":     ":memory:",
		"eim_user": ":memory:",
	}
	mdb, err := NewMultiDB("sqlite", ":memory:", databases)
	if err != nil {
		t.Fatalf("NewMultiDB failed: %v", err)
	}
	defer mdb.Close()

	shared := mdb.Shared()
	if shared == nil {
		t.Fatal("Shared() returned nil")
	}

	if !mdb.HasPerSchemaPools() {
		t.Fatal("HasPerSchemaPools() = false; want true (2 per-schema pools configured)")
	}

	// ForSchema("user") should return the user pool (NOT shared)
	userPool := mdb.ForSchema("user")
	if userPool == shared {
		t.Error("ForSchema(user) returned shared pool; want per-schema pool")
	}

	// ForSchema("eim:user") should return the eim_user pool (canonical form)
	eimUserPool := mdb.ForSchema("eim:user")
	if eimUserPool == shared {
		t.Error("ForSchema(eim:user) returned shared pool; want per-schema pool")
	}
	if eimUserPool != userPool {
		// eim_user and user are different schemas → different pools
		// (both are :memory: SQLite, but the *sql.DB instances are distinct)
	}

	// ForSchema("eim_user") should also work (alias normalization)
	eimUserPool2 := mdb.ForSchema("eim_user")
	if eimUserPool2 != eimUserPool {
		t.Error("ForSchema(eim_user) != ForSchema(eim:user); want same pool (alias)")
	}

	// ForSchema("live") should fall back to shared (not configured)
	livePool := mdb.ForSchema("live")
	if livePool != shared {
		t.Error("ForSchema(live) returned non-shared pool; want shared (fallback)")
	}

	// ForSchema("chat") should fall back to shared
	chatPool := mdb.ForSchema("chat")
	if chatPool != shared {
		t.Error("ForSchema(chat) returned non-shared pool; want shared (fallback)")
	}
}

// TestMultiDBForTable verifies the ForTable method using a SchemaShorter
// function (simulates model.SchemaShort).
func TestMultiDBForTable(t *testing.T) {
	// SchemaShorter stub: maps table → schema short-name
	shortener := func(table string) string {
		switch table {
		case "user", "user_grow", "room_gift_rank":
			return "user"
		case "live_room", "anchors", "match_schedule":
			return "live"
		case "chat_room_message":
			return "chat"
		case "eim_user":
			return "eim:user"
		default:
			return ""
		}
	}

	databases := map[string]string{
		"user":     ":memory:",
		"eim_user": ":memory:",
	}
	mdb, err := NewMultiDB("sqlite", ":memory:", databases)
	if err != nil {
		t.Fatalf("NewMultiDB failed: %v", err)
	}
	defer mdb.Close()

	shared := mdb.Shared()
	userPool := mdb.ForSchema("user")
	eimUserPool := mdb.ForSchema("eim:user")

	// Tables in the "user" schema → user pool
	for _, table := range []string{"user", "user_grow", "room_gift_rank"} {
		if got := mdb.ForTable(table, shortener); got != userPool {
			t.Errorf("ForTable(%q) did not return user pool", table)
		}
	}

	// Tables in the "live" schema → shared (fallback, not configured)
	for _, table := range []string{"live_room", "anchors", "match_schedule"} {
		if got := mdb.ForTable(table, shortener); got != shared {
			t.Errorf("ForTable(%q) did not return shared pool (fallback)", table)
		}
	}

	// "chat_room_message" → shared (fallback)
	if got := mdb.ForTable("chat_room_message", shortener); got != shared {
		t.Error("ForTable(chat_room_message) did not return shared pool (fallback)")
	}

	// "eim_user" → eim:user pool
	if got := mdb.ForTable("eim_user", shortener); got != eimUserPool {
		t.Error("ForTable(eim_user) did not return eim:user pool")
	}

	// Unknown table → shared (SchemaShorter returns "")
	if got := mdb.ForTable("unknown_table", shortener); got != shared {
		t.Error("ForTable(unknown_table) did not return shared pool")
	}

	// nil shortener → shared
	if got := mdb.ForTable("user", nil); got != shared {
		t.Error("ForTable with nil shortener did not return shared pool")
	}
}

// TestMultiDBNilSafe verifies that ForSchema on a nil MultiDB returns nil
// (prevents panics in edge cases).
func TestMultiDBNilSafe(t *testing.T) {
	var mdb *MultiDB
	if got := mdb.ForSchema("user"); got != nil {
		t.Errorf("nil MultiDB ForSchema() = %v; want nil", got)
	}
	if got := mdb.Shared(); got != nil {
		t.Errorf("nil MultiDB Shared() = %v; want nil", got)
	}
	mdb.Close() // should not panic
}

// TestMultiDBEmptyDatabasesMap verifies that an empty (but non-nil) Databases
// map behaves like shared-pool mode.
func TestMultiDBEmptyDatabasesMap(t *testing.T) {
	mdb, err := NewMultiDB("sqlite", ":memory:", map[string]string{})
	if err != nil {
		t.Fatalf("NewMultiDB failed: %v", err)
	}
	defer mdb.Close()

	if mdb.HasPerSchemaPools() {
		t.Error("HasPerSchemaPools() = true; want false (empty Databases map)")
	}

	shared := mdb.Shared()
	for _, schema := range []string{"user", "live", "chat", "eim:user"} {
		if got := mdb.ForSchema(schema); got != shared {
			t.Errorf("ForSchema(%q) != Shared() with empty Databases map", schema)
		}
	}
}

// TestMultiDBSkipsEmptyEntries verifies that empty DSN values in the Databases
// map are skipped (don't cause errors).
func TestMultiDBSkipsEmptyEntries(t *testing.T) {
	databases := map[string]string{
		"user":    ":memory:",
		"live":    "",  // empty → skipped
		"chat":    "  ", // whitespace → skipped
		"eim_user": ":memory:",
	}
	mdb, err := NewMultiDB("sqlite", ":memory:", databases)
	if err != nil {
		t.Fatalf("NewMultiDB failed: %v", err)
	}
	defer mdb.Close()

	// Should have 2 per-schema pools (user + eim_user), NOT 4
	// (live and chat were empty/whitespace → skipped)
	if !mdb.HasPerSchemaPools() {
		t.Fatal("HasPerSchemaPools() = false; want true")
	}

	shared := mdb.Shared()
	// user → per-schema pool
	if mdb.ForSchema("user") == shared {
		t.Error("ForSchema(user) returned shared; want per-schema")
	}
	// live → shared (was skipped)
	if mdb.ForSchema("live") != shared {
		t.Error("ForSchema(live) did not return shared; want shared (entry was empty/skipped)")
	}
	// chat → shared (was skipped)
	if mdb.ForSchema("chat") != shared {
		t.Error("ForSchema(chat) did not return shared; want shared (entry was whitespace/skipped)")
	}
	// eim_user → per-schema pool
	if mdb.ForSchema("eim:user") == shared {
		t.Error("ForSchema(eim:user) returned shared; want per-schema")
	}
}
