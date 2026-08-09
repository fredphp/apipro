package model

import "testing"

// TestTblMainAppSchemas verifies that the main app schemas (zb_*) resolve
// to fully-qualified table names with the configured prefix.
func TestTblMainAppSchemas(t *testing.T) {
	// Reset to defaults (other tests may have changed these).
	SetSchemaPrefix("zb_")
	SetEimSchemaPrefix("eim_")
	useSchemaPrefix = true

	cases := []struct {
		table string
		want  string
	}{
		{"user", "zb_user.user"},
		{"user_grow", "zb_user.user_grow"},
		{"room_gift_rank", "zb_user.room_gift_rank"},
		{"live_room", "zb_live.live_room"},
		{"anchors", "zb_live.anchors"},
		{"match_schedule", "zb_live.match_schedule"},
		{"match_schedule_room", "zb_live.match_schedule_room"},
		{"live_type", "zb_live.live_type"},
		{"live_hot_recommend", "zb_live.live_hot_recommend"},
		{"chat_room_message", "zb_chat.chat_room_message"},
		{"gift", "zb_gift.gift"},
		{"admin_user", "zb_admin.admin_user"},
		{"article", "zb_sys.article"},
		{"advertising", "zb_sys.advertising"},
		{"system_chat_message", "zb_sys.system_chat_message"},
		{"system_notice_message", "zb_sys.system_notice_message"},
		{"high_light", "zb_live.high_light"},
		{"live_stat", "zb_live.live_stat"},
		{"uid_pool", "zb_user.uid_pool"},
		{"account", "zb_user.account"},
		{"attention_compere", "zb_user.attention_compere"},
		{"login_log", "zb_user.login_log"},
		{"reg_info", "zb_user.reg_info"},
	}
	for _, c := range cases {
		got := Tbl(c.table)
		if got != c.want {
			t.Errorf("Tbl(%q) = %q; want %q", c.table, got, c.want)
		}
	}
}

// TestTblBasketballFootball verifies that basketball/football tables resolve
// to the zb_basketball / zb_football schemas.
func TestTblBasketballFootball(t *testing.T) {
	SetSchemaPrefix("zb_")
	SetEimSchemaPrefix("eim_")
	useSchemaPrefix = true

	cases := []struct {
		table string
		want  string
	}{
		{"basketball_team", "zb_basketball.basketball_team"},
		{"basketball_league", "zb_basketball.basketball_league"},
		{"basketball_stand", "zb_basketball.basketball_stand"},
		{"basketball_player", "zb_basketball.basketball_player"},
		{"football_team", "zb_football.football_team"},
		{"football_league", "zb_football.football_league"},
		{"football_stand", "zb_football.football_stand"},
		{"football_player", "zb_football.football_player"},
	}
	for _, c := range cases {
		got := Tbl(c.table)
		if got != c.want {
			t.Errorf("Tbl(%q) = %q; want %q", c.table, got, c.want)
		}
	}
}

// TestTblEimSchemas verifies that eim_* tables resolve to the eim_* schemas
// using the EIM prefix (not the main zb_ prefix).
func TestTblEimSchemas(t *testing.T) {
	SetSchemaPrefix("zb_")
	SetEimSchemaPrefix("eim_")
	useSchemaPrefix = true

	cases := []struct {
		table string
		want  string
	}{
		{"eim_user", "eim_user.eim_user"},
		{"eim_user_online", "eim_user.eim_user_online"},
		{"eim_friend", "eim_friend.eim_friend"},
		{"eim_friend_group", "eim_friend.eim_friend_group"},
		{"eim_friend_apply", "eim_friend.eim_friend_apply"},
		{"eim_group", "eim_group.eim_group"},
		{"eim_group_member", "eim_group.eim_group_member"},
		{"eim_group_msg", "eim_group.eim_group_msg"},
		{"eim_admin", "eim_admin.eim_admin"},
		{"eim_app_config", "eim_admin.eim_app_config"},
	}
	for _, c := range cases {
		got := Tbl(c.table)
		if got != c.want {
			t.Errorf("Tbl(%q) = %q; want %q", c.table, got, c.want)
		}
	}
}

// TestEimSchemaPrefixOverride verifies that SetEimSchemaPrefix changes the
// EIM schema prefix without affecting the main app prefix.
func TestEimSchemaPrefixOverride(t *testing.T) {
	SetSchemaPrefix("zb_")
	SetEimSchemaPrefix("im_") // custom EIM prefix
	useSchemaPrefix = true

	// Main app schemas keep the zb_ prefix
	if got := Tbl("user"); got != "zb_user.user" {
		t.Errorf("Tbl(user) = %q; want zb_user.user (main prefix unchanged)", got)
	}
	if got := Tbl("live_room"); got != "zb_live.live_room" {
		t.Errorf("Tbl(live_room) = %q; want zb_live.live_room (main prefix unchanged)", got)
	}

	// EIM schemas use the custom prefix
	if got := Tbl("eim_user"); got != "im_user.eim_user" {
		t.Errorf("Tbl(eim_user) = %q; want im_user.eim_user (custom EIM prefix)", got)
	}
	if got := Tbl("eim_friend"); got != "im_friend.eim_friend" {
		t.Errorf("Tbl(eim_friend) = %q; want im_friend.eim_friend (custom EIM prefix)", got)
	}

	// Reset
	SetEimSchemaPrefix("eim_")
}

// TestSchemaPrefixOverride verifies that SetSchemaPrefix changes the main app
// prefix (e.g. to "haima_" for backend-zero upstream) without affecting EIM.
func TestSchemaPrefixOverride(t *testing.T) {
	SetSchemaPrefix("haima_")
	SetEimSchemaPrefix("eim_")
	useSchemaPrefix = true

	// Main app schemas use haima_ prefix
	if got := Tbl("user"); got != "haima_user.user" {
		t.Errorf("Tbl(user) = %q; want haima_user.user", got)
	}
	if got := Tbl("live_room"); got != "haima_live.live_room" {
		t.Errorf("Tbl(live_room) = %q; want haima_live.live_room", got)
	}
	if got := Tbl("basketball_team"); got != "haima_basketball.basketball_team" {
		t.Errorf("Tbl(basketball_team) = %q; want haima_basketball.basketball_team", got)
	}

	// EIM schemas keep the eim_ prefix (independent)
	if got := Tbl("eim_user"); got != "eim_user.eim_user" {
		t.Errorf("Tbl(eim_user) = %q; want eim_user.eim_user (EIM prefix independent)", got)
	}

	// Reset
	SetSchemaPrefix("zb_")
}

// TestSchemaHelpers verifies the SchemaForTable / SchemaUser / SchemaLive /
// SchemaEim* helpers return fully-qualified schema names.
func TestSchemaHelpers(t *testing.T) {
	SetSchemaPrefix("zb_")
	SetEimSchemaPrefix("eim_")
	useSchemaPrefix = true

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"SchemaUser", SchemaUser(), "zb_user"},
		{"SchemaLive", SchemaLive(), "zb_live"},
		{"SchemaChat", SchemaChat(), "zb_chat"},
		{"SchemaGift", SchemaGift(), "zb_gift"},
		{"SchemaAdmin", SchemaAdmin(), "zb_admin"},
		{"SchemaSys", SchemaSys(), "zb_sys"},
		{"SchemaBasketball", SchemaBasketball(), "zb_basketball"},
		{"SchemaFootball", SchemaFootball(), "zb_football"},
		{"SchemaEimUser", SchemaEimUser(), "eim_user"},
		{"SchemaEimFriend", SchemaEimFriend(), "eim_friend"},
		{"SchemaEimGroup", SchemaEimGroup(), "eim_group"},
		{"SchemaEimAdmin", SchemaEimAdmin(), "eim_admin"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s() = %q; want %q", c.name, c.got, c.want)
		}
	}
}

// TestSchemaShort verifies the SchemaShort helper used by MultiDB.ForTable.
func TestSchemaShort(t *testing.T) {
	SetSchemaPrefix("zb_")
	SetEimSchemaPrefix("eim_")
	useSchemaPrefix = true

	cases := []struct {
		table string
		want  string
	}{
		{"user", "user"},
		{"live_room", "live"},
		{"chat_room_message", "chat"},
		{"gift", "gift"},
		{"admin_user", "admin"},
		{"article", "sys"},
		{"basketball_team", "basketball"},
		{"football_team", "football"},
		{"eim_user", "eim:user"},
		{"eim_friend", "eim:friend"},
		{"eim_group", "eim:group"},
		{"eim_admin", "eim:admin"},
		{"unknown_table", ""}, // unknown → empty (fallback to shared pool)
	}
	for _, c := range cases {
		got := SchemaShort(c.table)
		if got != c.want {
			t.Errorf("SchemaShort(%q) = %q; want %q", c.table, got, c.want)
		}
	}
}

// TestSQLiteMode verifies that SetNoSchemaPrefix makes Tbl() return bare names.
func TestSQLiteMode(t *testing.T) {
	SetNoSchemaPrefix() // SQLite mode

	cases := []string{"user", "live_room", "eim_user", "eim_friend", "basketball_team"}
	for _, table := range cases {
		if got := Tbl(table); got != table {
			t.Errorf("Tbl(%q) in SQLite mode = %q; want %q (bare name)", table, got, table)
		}
		if got := SchemaForTable(table); got != "" {
			t.Errorf("SchemaForTable(%q) in SQLite mode = %q; want empty", table, got)
		}
	}

	// Reset to MySQL mode for other tests
	useSchemaPrefix = true
	SetSchemaPrefix("zb_")
	SetEimSchemaPrefix("eim_")
}

// TestUnknownTableFallback verifies that unknown tables fall back to bare names.
func TestUnknownTableFallback(t *testing.T) {
	SetSchemaPrefix("zb_")
	SetEimSchemaPrefix("eim_")
	useSchemaPrefix = true

	if got := Tbl("nonexistent_table"); got != "nonexistent_table" {
		t.Errorf("Tbl(unknown) = %q; want bare name 'nonexistent_table'", got)
	}
}
