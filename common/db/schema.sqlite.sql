-- =====================================================================
-- apipro SQLite schema (dev / self-check, embedded copy)
-- Mirror of deploy/schema.sqlite.sql. Auto-applied on startup when
-- DBDriver=sqlite. Single-file layout — all tables live in one .db file;
-- see deploy/schema.mysql.sql for the production multi-database layout
-- (zb_user / zb_live / zb_chat / ...).
-- =====================================================================

-- (in zb_user) registered users + guests (password = client md5 hash)
CREATE TABLE IF NOT EXISTS user (
  uid          INTEGER PRIMARY KEY,
  login_name   TEXT NOT NULL DEFAULT '',
  nick_name    TEXT NOT NULL DEFAULT '',
  phone        TEXT NOT NULL DEFAULT '',
  country_code TEXT NOT NULL DEFAULT '',
  password     TEXT NOT NULL DEFAULT '',
  salt         TEXT NOT NULL DEFAULT '',
  pwd_type     INTEGER NOT NULL DEFAULT 2,
  user_type    INTEGER NOT NULL DEFAULT 1,
  score        INTEGER NOT NULL DEFAULT 0,
  grow         INTEGER NOT NULL DEFAULT 0,
  status       INTEGER NOT NULL DEFAULT 1,
  icon         TEXT NOT NULL DEFAULT '',
  gender       INTEGER NOT NULL DEFAULT 0,
  birthday     TEXT,
  plat         INTEGER NOT NULL DEFAULT 0,
  created_at   INTEGER NOT NULL DEFAULT 0,
  updated_at   INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_phone ON user(country_code, phone);
CREATE UNIQUE INDEX IF NOT EXISTS uk_loginname ON user(login_name);

-- (in zb_user) user grow level lookup
CREATE TABLE IF NOT EXISTS user_grow (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  name      TEXT NOT NULL DEFAULT '',
  min_grow  INTEGER NOT NULL DEFAULT 0,
  sort      INTEGER NOT NULL DEFAULT 0,
  status    INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_min_grow ON user_grow(min_grow);

-- (in zb_live) live type catalog (top-level + child)
CREATE TABLE IF NOT EXISTS live_type (
  live_type_id INTEGER PRIMARY KEY,
  parent_id    INTEGER NOT NULL DEFAULT 0,
  type_name    TEXT NOT NULL DEFAULT '',
  icon         TEXT NOT NULL DEFAULT '',
  status       INTEGER NOT NULL DEFAULT 1,
  sort         INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_lt_parent_status ON live_type(parent_id, status);

-- (in zb_live) anchor extra profile (base user info lives in zb_user.user)
CREATE TABLE IF NOT EXISTS anchors (
  uid          INTEGER PRIMARY KEY,
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

-- (in zb_live) live rooms
CREATE TABLE IF NOT EXISTS live_room (
  uid                       INTEGER NOT NULL,
  room_num                  TEXT PRIMARY KEY,
  title                     TEXT NOT NULL DEFAULT '',
  contact                   TEXT NOT NULL DEFAULT '',
  cover                     TEXT NOT NULL DEFAULT '',
  custom_cover              TEXT NOT NULL DEFAULT '',
  notice                    TEXT NOT NULL DEFAULT '',
  detail                    TEXT NOT NULL DEFAULT '',
  live_flv                  TEXT NOT NULL DEFAULT '',
  live_m3u8                 TEXT NOT NULL DEFAULT '',
  live_status               INTEGER NOT NULL DEFAULT 2,
  room_status               INTEGER NOT NULL DEFAULT 1,
  live_type                 INTEGER NOT NULL DEFAULT 0,
  live_type_parent          INTEGER NOT NULL DEFAULT 0,
  focus_count               INTEGER NOT NULL DEFAULT 0,
  fictitious_focus_count    INTEGER NOT NULL DEFAULT 0,
  visit_count               INTEGER NOT NULL DEFAULT 0,
  fictitious_visit_count    INTEGER NOT NULL DEFAULT 0,
  mark_type                 INTEGER NOT NULL DEFAULT 0,
  assistant_uid             INTEGER NOT NULL DEFAULT 0,
  hd                        INTEGER NOT NULL DEFAULT 0,
  stream_type               INTEGER NOT NULL DEFAULT 0,
  push_content              TEXT NOT NULL DEFAULT '',
  created_at                INTEGER NOT NULL DEFAULT 0,
  updated_at                INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_room_anchor ON live_room(uid);
CREATE INDEX IF NOT EXISTS idx_room_live ON live_room(live_status, room_status);
CREATE INDEX IF NOT EXISTS idx_room_ltp ON live_room(live_type_parent);

-- (in zb_live) match schedule (赛程)
CREATE TABLE IF NOT EXISTS match_schedule (
  schedule_id        INTEGER PRIMARY KEY,
  host_name          TEXT NOT NULL DEFAULT '',
  guest_name         TEXT NOT NULL DEFAULT '',
  host_score         INTEGER,
  guest_score        INTEGER,
  match_time         TEXT,
  live_type          INTEGER NOT NULL DEFAULT 0,
  live_type_parent   INTEGER NOT NULL DEFAULT 0,
  host_icon          TEXT NOT NULL DEFAULT '',
  guest_icon         TEXT NOT NULL DEFAULT '',
  sub_type_name      TEXT NOT NULL DEFAULT '',
  match_status       INTEGER,
  status             INTEGER NOT NULL DEFAULT 1,
  hot                INTEGER NOT NULL DEFAULT 0,
  green_match        INTEGER NOT NULL DEFAULT 0,
  created_at         INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_match_time ON match_schedule(match_time);
CREATE INDEX IF NOT EXISTS idx_match_ltp ON match_schedule(live_type_parent);
CREATE INDEX IF NOT EXISTS idx_match_status ON match_schedule(status);

-- (in zb_live) match ↔ room link
CREATE TABLE IF NOT EXISTS match_schedule_room (
  schedule_id  INTEGER NOT NULL,
  room_num     TEXT NOT NULL,
  status       INTEGER NOT NULL DEFAULT 1,
  PRIMARY KEY (schedule_id, room_num)
);
CREATE INDEX IF NOT EXISTS idx_msr_room ON match_schedule_room(room_num);

-- (in zb_live) homepage hot rooms
CREATE TABLE IF NOT EXISTS live_hot_recommend (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  room_num    TEXT NOT NULL,
  room_json   TEXT,
  sort_order  INTEGER NOT NULL DEFAULT 0,
  status      INTEGER NOT NULL DEFAULT 1,
  begin_time  TEXT,
  end_time    TEXT
);
CREATE INDEX IF NOT EXISTS idx_hot_room ON live_hot_recommend(room_num);

-- (in zb_user) room gift contribution leaderboard
CREATE TABLE IF NOT EXISTS room_gift_rank (
  room_num       TEXT NOT NULL,
  uid            INTEGER NOT NULL,
  nick_name      TEXT NOT NULL DEFAULT '',
  icon           TEXT NOT NULL DEFAULT '',
  score          INTEGER NOT NULL DEFAULT 0,
  rank_no        INTEGER NOT NULL DEFAULT 0,
  last_send_time TEXT,
  PRIMARY KEY (room_num, uid)
);
CREATE INDEX IF NOT EXISTS idx_rgr_rank ON room_gift_rank(room_num, rank_no);

-- (in zb_chat) live-room chat history
CREATE TABLE IF NOT EXISTS chat_room_message (
  chat_room_message_id INTEGER PRIMARY KEY,
  send_uid             INTEGER NOT NULL DEFAULT 0,
  room_num             TEXT NOT NULL DEFAULT '',
  send_time            TEXT,
  content              TEXT,
  type                 INTEGER NOT NULL DEFAULT 1,
  ip                   TEXT NOT NULL DEFAULT '',
  status               INTEGER NOT NULL DEFAULT 1,
  created_at           INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_crm_room_time ON chat_room_message(room_num, send_time);

-- =====================================================================
-- Seed (same as MySQL schema, with SQLite syntax)
-- =====================================================================

INSERT OR IGNORE INTO live_type (live_type_id,parent_id,type_name,icon,status,sort) VALUES
 (1, 0, 'Bóng đá',   'https://sta.ncctrials.com/file/ico/football.png',   1, 100),
 (2, 0, 'Bóng rổ',   'https://sta.ncctrials.com/file/ico/basketball.png', 1, 90),
 (5, 0, 'Analysis',  'https://sta.ncctrials.com/file/ico/analysis.png',   1, 70);

INSERT OR IGNORE INTO user (uid,login_name,nick_name,phone,country_code,password,salt,pwd_type,user_type,score,grow,status,icon,gender,plat,created_at,updated_at) VALUES
 (1001, 'anchor1001', 'Cá Mực FM', '13800138001', '86', '', '', 2, 2, 0, 100, 1, 'https://sta.ncctrials.com/file/avatar/a1001.png', 1, 4, strftime('%s','now'), strftime('%s','now')),
 (1002, 'anchor1002', 'NBA Boy',   '13800138002', '86', '', '', 2, 2, 0, 200, 1, 'https://sta.ncctrials.com/file/avatar/a1002.png', 1, 4, strftime('%s','now'), strftime('%s','now')),
 (1003, 'anchor1003', 'La Liga',   '13800138003', '86', '', '', 2, 2, 0, 300, 1, 'https://sta.ncctrials.com/file/avatar/a1003.png', 1, 4, strftime('%s','now'), strftime('%s','now'));

INSERT OR IGNORE INTO anchors (uid,nick_name,icon,cut_out_icon,intro,fans,follow,hot,room_num,detail,notice,live,created_at) VALUES
 (1001, 'Cá Mực FM', 'https://sta.ncctrials.com/file/avatar/a1001.png', '', 'Ngoại hạng Anh mỗi tối', 128000, 98000, 9527, '1001', 'Livestream Ngoại hạng Anh mỗi tối 20:00', 'No spam', 1, strftime('%s','now')),
 (1002, 'NBA Boy',   'https://sta.ncctrials.com/file/avatar/a1002.png', '', 'NBA / CBA analyst',       86000,  64000, 7610, '1002', 'NBA stream',                          'Be civil', 1, strftime('%s','now')),
 (1003, 'La Liga',   'https://sta.ncctrials.com/file/avatar/a1003.png', '', 'La Liga tactics',         54000,  41000, 5230, '1003', 'La Liga live',                       'Rational talk', 1, strftime('%s','now'));

INSERT OR IGNORE INTO live_room (uid,room_num,title,contact,cover,custom_cover,notice,detail,live_flv,live_m3u8,live_status,room_status,live_type,live_type_parent,focus_count,fictitious_focus_count,visit_count,fictitious_visit_count,mark_type,assistant_uid,hd,stream_type,push_content,created_at,updated_at) VALUES
 (1001, '1001', 'Ngoại hạng Anh: MU vs Liverpool', 'skype:anchor1001', 'https://sta.ncctrials.com/file/cover/1001.jpg', '', 'No spam', 'MU vs LIV 20:00', '', 'https://live.zbyy.example/1001/hd.m3u8', 1, 1, 0, 1, 12000, 5000, 28000, 8000, 3, 0, 1, 7, 'Welcome to room 1001', strftime('%s','now'), strftime('%s','now')),
 (1002, '1002', 'NBA: Lakers vs Warriors',         'skype:anchor1002', 'https://sta.ncctrials.com/file/cover/1002.jpg', '', 'Be civil', 'LAL vs GSW',       '', 'https://live.zbyy.example/1002/hd.m3u8', 1, 1, 0, 2,  9000, 3000, 41000, 9000, 2, 0, 1, 7, 'Welcome to room 1002', strftime('%s','now'), strftime('%s','now')),
 (1003, '1003', 'La Liga: Real vs Barca',           'skype:anchor1003', 'https://sta.ncctrials.com/file/cover/1003.jpg', '', 'Rational talk', 'RMA vs BAR',   '', '',                                        2, 1, 0, 1,  5000, 1000,  8000, 2000, 1, 0, 0, 7, 'Welcome to room 1003', strftime('%s','now'), strftime('%s','now'));

INSERT OR IGNORE INTO match_schedule (schedule_id,host_name,guest_name,host_score,guest_score,match_time,live_type,live_type_parent,host_icon,guest_icon,sub_type_name,match_status,status,hot,green_match,created_at) VALUES
 (980754, 'MU',        'Liverpool', 0, 0, strftime('%Y-%m-%d %H:%M:%S','now'),                      0, 1, 'https://sta.ncctrials.com/file/team/man.png', 'https://sta.ncctrials.com/file/team/liv.png', 'EPL',     NULL, 1, 100, 0, strftime('%s','now')),
 (980755, 'Lakers',    'Warriors',  0, 0, strftime('%Y-%m-%d %H:%M:%S','now','+1 hour'),            0, 2, 'https://sta.ncctrials.com/file/team/lal.png', 'https://sta.ncctrials.com/file/team/gsw.png', 'NBA',     NULL, 1,  90, 0, strftime('%s','now')),
 (980756, 'Real',      'Barca',     0, 0, strftime('%Y-%m-%d %H:%M:%S','now','+2 hours'),           0, 1, 'https://sta.ncctrials.com/file/team/rma.png', 'https://sta.ncctrials.com/file/team/bar.png', 'La Liga', NULL, 1,  80, 0, strftime('%s','now')),
 (980757, 'Analysis',  'Show',      0, 0, strftime('%Y-%m-%d %H:%M:%S','now','+3 hours'),           0, 5, 'https://sta.ncctrials.com/file/team/ana.png', 'https://sta.ncctrials.com/file/team/sho.png', 'Talk',    NULL, 1,  60, 0, strftime('%s','now'));

INSERT OR IGNORE INTO match_schedule_room (schedule_id,room_num,status) VALUES
 (980754, '1001', 1),
 (980755, '1002', 1),
 (980756, '1003', 1),
 (980757, '1001', 1);

INSERT OR IGNORE INTO live_hot_recommend (room_num,room_json,sort_order,status,begin_time,end_time) VALUES
 ('1001', '[{"uid":1001,"roomNum":"1001","nickName":"Cá Mực FM","sort":1}]', 1, 1, strftime('%Y-%m-%d %H:%M:%S','now','-1 day'), strftime('%Y-%m-%d %H:%M:%S','now','+30 day')),
 ('1002', '[{"uid":1002,"roomNum":"1002","nickName":"NBA Boy","sort":2}]',   2, 1, strftime('%Y-%m-%d %H:%M:%S','now','-1 day'), strftime('%Y-%m-%d %H:%M:%S','now','+30 day'));

INSERT OR IGNORE INTO room_gift_rank (room_num,uid,nick_name,icon,score,rank_no) VALUES
 ('1001', 5001, 'demo_user', 'https://sta.ncctrials.com/file/avatar/demo.png', 18820, 1),
 ('1001', 1001, 'Cá Mực FM', 'https://sta.ncctrials.com/file/avatar/a1001.png', 12330, 2),
 ('1002', 1002, 'NBA Boy',   'https://sta.ncctrials.com/file/avatar/a1002.png', 22110, 1);

-- Demo audience user — same as MySQL schema:
--   phone: 13800138888, country_code: 86, plain password: qwe123
--   client sends md5("qwe123") = "200820e3227815ed1756a6b531e7e0d2"
--   salt: "7Whd1U2T1pjeDP4HcSVDxwBMF5Vf6NWx"
--   stored password = md5("200820e3227815ed1756a6b531e7e0d2" + "7Whd1U2T1pjeDP4HcSVDxwBMF5Vf6NWx")
--                  = "8ec733b6de4825a437faee2c01ddd309"
INSERT OR IGNORE INTO user (uid,login_name,nick_name,phone,country_code,password,salt,pwd_type,user_type,score,grow,status,icon,gender,plat,created_at,updated_at) VALUES
 (5001, 'demo', 'demo_user', '13800138888', '86', '8ec733b6de4825a437faee2c01ddd309', '7Whd1U2T1pjeDP4HcSVDxwBMF5Vf6NWx', 2, 1, 500, 0, 1, 'https://sta.ncctrials.com/file/avatar/demo.png', 1, 4, strftime('%s','now'), strftime('%s','now'));
