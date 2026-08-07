package main

// Embedded SQLite schema (same as deploy/schema.sqlite.sql but as a Go string
// so the seed tool can auto-create tables without reading external files).

const sqliteSchema = `
CREATE TABLE IF NOT EXISTS users (
  uid          TEXT PRIMARY KEY,
  login_name   TEXT NOT NULL DEFAULT '',
  nick_name    TEXT NOT NULL DEFAULT '',
  phone        TEXT NOT NULL DEFAULT '',
  country_code TEXT NOT NULL DEFAULT '',
  password     TEXT NOT NULL DEFAULT '',
  pwd_type     INTEGER NOT NULL DEFAULT 1,
  grow         INTEGER NOT NULL DEFAULT 0,
  score        INTEGER NOT NULL DEFAULT 0,
  level        INTEGER NOT NULL DEFAULT 1,
  avatar       TEXT NOT NULL DEFAULT '',
  is_user      INTEGER NOT NULL DEFAULT 1,
  created_at   INTEGER NOT NULL DEFAULT 0,
  updated_at   INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_phone ON users(country_code, phone);
CREATE UNIQUE INDEX IF NOT EXISTS uk_loginname ON users(login_name);

CREATE TABLE IF NOT EXISTS live_types (
  code       TEXT PRIMARY KEY,
  name       TEXT NOT NULL DEFAULT '',
  icon       TEXT NOT NULL DEFAULT '',
  sort_order INTEGER NOT NULL DEFAULT 0,
  status     INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS anchors (
  uid          TEXT PRIMARY KEY,
  nick_name    TEXT NOT NULL DEFAULT '',
  icon         TEXT NOT NULL DEFAULT '',
  cut_out_icon TEXT NOT NULL DEFAULT '',
  intro        TEXT NOT NULL DEFAULT '',
  fans         INTEGER NOT NULL DEFAULT 0,
  follow       INTEGER NOT NULL DEFAULT 0,
  hot          INTEGER NOT NULL DEFAULT 0,
  room_num     TEXT NOT NULL DEFAULT '',
  detail       TEXT NOT NULL DEFAULT '',
  notice       TEXT NOT NULL DEFAULT '',
  live         INTEGER NOT NULL DEFAULT 0,
  created_at   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_anchor_room ON anchors(room_num);
CREATE INDEX IF NOT EXISTS idx_anchor_hot ON anchors(hot);

CREATE TABLE IF NOT EXISTS rooms (
  room_num    TEXT PRIMARY KEY,
  title       TEXT NOT NULL DEFAULT '',
  cover       TEXT NOT NULL DEFAULT '',
  live        INTEGER NOT NULL DEFAULT 0,
  view_num    INTEGER NOT NULL DEFAULT 0,
  live_type   TEXT NOT NULL DEFAULT '',
  anchor_uid  TEXT NOT NULL DEFAULT '',
  stream_urls TEXT NOT NULL DEFAULT '[]',
  notice      TEXT NOT NULL DEFAULT '',
  tags        TEXT NOT NULL DEFAULT '[]',
  cate_name   TEXT NOT NULL DEFAULT '',
  created_at  INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_room_anchor ON rooms(anchor_uid);
CREATE INDEX IF NOT EXISTS idx_room_live ON rooms(live);

CREATE TABLE IF NOT EXISTS matches (
  schedule_id        TEXT PRIMARY KEY,
  sub_cate_name      TEXT NOT NULL DEFAULT '',
  cate_name          TEXT NOT NULL DEFAULT '',
  match_time         TEXT NOT NULL DEFAULT '',
  match_date         TEXT NOT NULL DEFAULT '',
  host_name          TEXT NOT NULL DEFAULT '',
  host_icon          TEXT NOT NULL DEFAULT '',
  guest_name         TEXT NOT NULL DEFAULT '',
  guest_icon         TEXT NOT NULL DEFAULT '',
  venue              TEXT NOT NULL DEFAULT '',
  status             TEXT NOT NULL DEFAULT 'not_started',
  reservation_status INTEGER NOT NULL DEFAULT 0,
  created_at         INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_match_date ON matches(match_date);
CREATE INDEX IF NOT EXISTS idx_match_cate ON matches(cate_name);
CREATE INDEX IF NOT EXISTS idx_match_status ON matches(status);

CREATE TABLE IF NOT EXISTS match_anchors (
  match_id   TEXT NOT NULL,
  anchor_uid TEXT NOT NULL,
  PRIMARY KEY (match_id, anchor_uid)
);
CREATE INDEX IF NOT EXISTS idx_ma_anchor ON match_anchors(anchor_uid);

CREATE TABLE IF NOT EXISTS room_ranks (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  room_num  TEXT NOT NULL,
  uid       TEXT NOT NULL DEFAULT '',
  nick_name TEXT NOT NULL DEFAULT '',
  icon      TEXT NOT NULL DEFAULT '',
  score     INTEGER NOT NULL DEFAULT 0,
  rank_no   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_rank_room ON room_ranks(room_num, rank_no);
`
