-- =====================================================================
-- apipro MySQL schema (production)
-- Matches the backend-zero YuYanTV data model.
-- Run:  mysql -u root -p < deploy/schema.mysql.sql
-- =====================================================================

CREATE DATABASE IF NOT EXISTS apipro DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE apipro;

-- ---------------------------------------------------------------------
-- users (audience + anchors; user_type distinguishes them)
--   password stores md5(md5(plain_password) + salt)  — pwd_type=2 only
--   salt = base64(32 random bytes) = 44 ASCII chars
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `user` (
  `uid`          BIGINT       NOT NULL COMMENT 'unique user id',
  `login_name`   VARCHAR(64)  NOT NULL DEFAULT '',
  `nick_name`    VARCHAR(64)  NOT NULL DEFAULT '',
  `phone`        VARCHAR(20)  NOT NULL DEFAULT '',
  `country_code` VARCHAR(8)   NOT NULL DEFAULT '' COMMENT 'normalized, no +, e.g. 86',
  `password`     VARCHAR(64)  NOT NULL DEFAULT '' COMMENT 'md5(client_md5+salt) lowercase hex',
  `salt`         VARCHAR(64)  NOT NULL DEFAULT '' COMMENT 'base64(32 random bytes), 44 chars',
  `pwd_type`     TINYINT      NOT NULL DEFAULT 2 COMMENT '2=md5(pwd)+salt (only supported)',
  `user_type`    TINYINT      NOT NULL DEFAULT 1 COMMENT '1=audience, 2=anchor, 3=admin',
  `score`        BIGINT       NOT NULL DEFAULT 0,
  `grow`         BIGINT       NOT NULL DEFAULT 0,
  `status`       TINYINT      NOT NULL DEFAULT 1 COMMENT '1=normal, 2/3=banned',
  `icon`         VARCHAR(255) NOT NULL DEFAULT '',
  `gender`       TINYINT      NOT NULL DEFAULT 0 COMMENT '0=unknown,1=male,2=female,3=other',
  `birthday`     DATETIME     NULL,
  `plat`         TINYINT      NOT NULL DEFAULT 0 COMMENT '1=android 2=ios 3=pc 4=h5',
  `created_at`   BIGINT       NOT NULL DEFAULT 0,
  `updated_at`   BIGINT       NOT NULL DEFAULT 0,
  PRIMARY KEY (`uid`),
  UNIQUE KEY `uk_phone` (`country_code`, `phone`),
  UNIQUE KEY `uk_loginname` (`login_name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='users';

-- ---------------------------------------------------------------------
-- user_grow (level lookup)
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `user_grow` (
  `id`             BIGINT       NOT NULL AUTO_INCREMENT,
  `name`           VARCHAR(32)  NOT NULL DEFAULT '',
  `min_grow`       BIGINT       NOT NULL DEFAULT 0,
  `sort`           INT          NOT NULL DEFAULT 0,
  `status`         TINYINT      NOT NULL DEFAULT 1,
  PRIMARY KEY (`id`),
  KEY `idx_min_grow` (`min_grow`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='user grow levels';

-- ---------------------------------------------------------------------
-- live_type (直播分类 — top-level + child)
--   parent_id=0 = top-level; child rows have parent_id pointing to a top-level row.
--   Top-level live_type_id: 1=football, 2=basketball, 5=analysis
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `live_type` (
  `live_type_id`   INT          NOT NULL COMMENT 'PK; top-level: 1,2,5; child: 20 (analysis child), etc.',
  `parent_id`      INT          NOT NULL DEFAULT 0 COMMENT '0=top-level',
  `type_name`      VARCHAR(64)  NOT NULL DEFAULT '' COMMENT 'Bóng đá / Bóng rổ / football etc.',
  `icon`           VARCHAR(255) NOT NULL DEFAULT '',
  `status`         TINYINT      NOT NULL DEFAULT 1,
  `sort`           INT          NOT NULL DEFAULT 0,
  PRIMARY KEY (`live_type_id`),
  KEY `idx_parent_status` (`parent_id`, `status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='live types';

-- ---------------------------------------------------------------------
-- anchors (anchor extra info — user table holds nick/icon; this holds
-- room_num, detail, notice, cut_out_icon)
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `anchors` (
  `uid`          BIGINT       NOT NULL COMMENT 'FK → user.uid',
  `nick_name`    VARCHAR(64)  NOT NULL DEFAULT '',
  `icon`         VARCHAR(255) NOT NULL DEFAULT '',
  `cut_out_icon` VARCHAR(255) NOT NULL DEFAULT '',
  `intro`        VARCHAR(500) NOT NULL DEFAULT '',
  `fans`         BIGINT       NOT NULL DEFAULT 0,
  `follow`       BIGINT       NOT NULL DEFAULT 0,
  `hot`          BIGINT       NOT NULL DEFAULT 0,
  `room_num`     VARCHAR(16)  NOT NULL DEFAULT '',
  `detail`       VARCHAR(500) NOT NULL DEFAULT '',
  `notice`       VARCHAR(500) NOT NULL DEFAULT '',
  `live`         TINYINT      NOT NULL DEFAULT 0,
  `created_at`   BIGINT       NOT NULL DEFAULT 0,
  PRIMARY KEY (`uid`),
  KEY `idx_room` (`room_num`),
  KEY `idx_hot` (`hot`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='anchor profile';

-- ---------------------------------------------------------------------
-- live_room (直播间)
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `live_room` (
  `uid`                       BIGINT       NOT NULL COMMENT 'FK → user.uid (anchor)',
  `room_num`                  VARCHAR(16)  NOT NULL,
  `title`                     VARCHAR(200) NOT NULL DEFAULT '',
  `contact`                   VARCHAR(128) NOT NULL DEFAULT '',
  `cover`                     VARCHAR(255) NOT NULL DEFAULT '',
  `custom_cover`              VARCHAR(255) NOT NULL DEFAULT '',
  `notice`                    VARCHAR(500) NOT NULL DEFAULT '',
  `detail`                    VARCHAR(500) NOT NULL DEFAULT '',
  `live_flv`                  VARCHAR(500) NOT NULL DEFAULT '',
  `live_m3u8`                 VARCHAR(500) NOT NULL DEFAULT '',
  `live_status`               TINYINT      NOT NULL DEFAULT 2 COMMENT '1=on-air, 2=off',
  `room_status`               TINYINT      NOT NULL DEFAULT 1 COMMENT '1=visible',
  `live_type`                 INT          NOT NULL DEFAULT 0 COMMENT 'child live_type_id',
  `live_type_parent`          INT          NOT NULL DEFAULT 0 COMMENT 'top-level: 1,2,5',
  `focus_count`               BIGINT       NOT NULL DEFAULT 0,
  `fictitious_focus_count`    BIGINT       NOT NULL DEFAULT 0,
  `visit_count`               BIGINT       NOT NULL DEFAULT 0,
  `fictitious_visit_count`    BIGINT       NOT NULL DEFAULT 0,
  `mark_type`                 INT          NOT NULL DEFAULT 0 COMMENT '1=official, 2=recommend, 3=hot',
  `assistant_uid`             BIGINT       NOT NULL DEFAULT 0,
  `hd`                        INT          NOT NULL DEFAULT 0,
  `stream_type`               INT          NOT NULL DEFAULT 0,
  `push_content`              VARCHAR(500) NOT NULL DEFAULT '',
  `created_at`                BIGINT       NOT NULL DEFAULT 0,
  `updated_at`                BIGINT       NOT NULL DEFAULT 0,
  PRIMARY KEY (`room_num`),
  KEY `idx_anchor` (`uid`),
  KEY `idx_live_status` (`live_status`, `room_status`),
  KEY `idx_live_type_parent` (`live_type_parent`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='live rooms';

-- ---------------------------------------------------------------------
-- match_schedule (赛程/比赛)
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `match_schedule` (
  `schedule_id`        BIGINT       NOT NULL,
  `host_name`          VARCHAR(64)  NOT NULL DEFAULT '',
  `guest_name`         VARCHAR(64)  NOT NULL DEFAULT '',
  `host_score`         INT          NULL,
  `guest_score`        INT          NULL,
  `match_time`         DATETIME     NULL COMMENT 'UTC DATETIME; served as int64 ms UTC',
  `live_type`          INT          NOT NULL DEFAULT 0 COMMENT 'child live_type_id',
  `live_type_parent`   INT          NOT NULL DEFAULT 0 COMMENT '1=football 2=basketball 5=analysis',
  `host_icon`          VARCHAR(255) NOT NULL DEFAULT '',
  `guest_icon`         VARCHAR(255) NOT NULL DEFAULT '',
  `sub_type_name`      VARCHAR(64)  NOT NULL DEFAULT '' COMMENT 'JSON: subCateName',
  `match_status`       INT          NULL,
  `status`             TINYINT      NOT NULL DEFAULT 1 COMMENT '1=enabled',
  `hot`                INT          NOT NULL DEFAULT 0,
  `green_match`        TINYINT      NOT NULL DEFAULT 0,
  `created_at`         BIGINT       NOT NULL DEFAULT 0,
  PRIMARY KEY (`schedule_id`),
  KEY `idx_match_time` (`match_time`),
  KEY `idx_live_type_parent` (`live_type_parent`),
  KEY `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='match schedule';

-- ---------------------------------------------------------------------
-- match_schedule_room (match ↔ room link)
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `match_schedule_room` (
  `schedule_id`   BIGINT       NOT NULL,
  `room_num`      VARCHAR(16)  NOT NULL,
  `status`        TINYINT      NOT NULL DEFAULT 1,
  PRIMARY KEY (`schedule_id`, `room_num`),
  KEY `idx_room` (`room_num`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='match ↔ room link';

-- ---------------------------------------------------------------------
-- live_hot_recommend (homepage hot rooms)
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `live_hot_recommend` (
  `id`          BIGINT       NOT NULL AUTO_INCREMENT,
  `room_num`    VARCHAR(16)  NOT NULL,
  `room_json`   TEXT         NULL COMMENT 'JSON [{uid, roomNum, nickName, sort}]',
  `sort_order`  INT          NOT NULL DEFAULT 0,
  `status`      TINYINT      NOT NULL DEFAULT 1,
  `begin_time`  DATETIME     NULL,
  `end_time`    DATETIME     NULL,
  PRIMARY KEY (`id`),
  KEY `idx_room` (`room_num`),
  KEY `idx_status_time` (`status`, `begin_time`, `end_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='live hot recommend';

-- ---------------------------------------------------------------------
-- room_gift_rank (room gift contribution leaderboard)
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `room_gift_rank` (
  `room_num`       VARCHAR(16)  NOT NULL,
  `uid`            BIGINT       NOT NULL,
  `nick_name`      VARCHAR(64)  NOT NULL DEFAULT '',
  `icon`           VARCHAR(255) NOT NULL DEFAULT '',
  `score`          BIGINT       NOT NULL DEFAULT 0 COMMENT 'contribution',
  `rank_no`        INT          NOT NULL DEFAULT 0,
  `last_send_time` DATETIME     NULL,
  PRIMARY KEY (`room_num`, `uid`),
  KEY `idx_rank` (`room_num`, `rank_no`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='room gift rank';

-- ---------------------------------------------------------------------
-- chat_room_message (live-room chat history)
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `chat_room_message` (
  `chat_room_message_id`  BIGINT       NOT NULL,
  `send_uid`              BIGINT       NOT NULL DEFAULT 0,
  `room_num`              VARCHAR(16)  NOT NULL DEFAULT '',
  `send_time`             DATETIME     NULL,
  `content`               TEXT         NULL,
  `type`                  TINYINT      NOT NULL DEFAULT 1 COMMENT '1=text 2=gift 3=system',
  `ip`                    VARCHAR(64)  NOT NULL DEFAULT '',
  `status`                TINYINT      NOT NULL DEFAULT 1 COMMENT '1=visible 0=deleted',
  `created_at`            BIGINT       NOT NULL DEFAULT 0,
  PRIMARY KEY (`chat_room_message_id`),
  KEY `idx_room_time` (`room_num`, `send_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='chat room messages';

-- =====================================================================
-- Seed data
-- =====================================================================

-- Live types (top-level: 1=football, 2=basketball, 5=analysis)
INSERT INTO `live_type` (`live_type_id`,`parent_id`,`type_name`,`icon`,`status`,`sort`) VALUES
 (1, 0, 'Bóng đá',   'https://sta.ncctrials.com/file/ico/football.png',   1, 100),
 (2, 0, 'Bóng rổ',   'https://sta.ncctrials.com/file/ico/basketball.png', 1, 90),
 (5, 0, 'Analysis',  'https://sta.ncctrials.com/file/ico/analysis.png',   1, 70)
ON DUPLICATE KEY UPDATE `type_name`=VALUES(`type_name`);

-- Anchor users (user_type=2)
INSERT INTO `user` (`uid`,`login_name`,`nick_name`,`phone`,`country_code`,`password`,`salt`,`pwd_type`,`user_type`,`score`,`grow`,`status`,`icon`,`gender`,`plat`,`created_at`,`updated_at`) VALUES
 (1001, 'anchor1001', 'Cá Mực FM', '13800138001', '86', '', '', 2, 2, 0, 100, 1, 'https://sta.ncctrials.com/file/avatar/a1001.png', 1, 4, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
 (1002, 'anchor1002', 'NBA Boy',   '13800138002', '86', '', '', 2, 2, 0, 200, 1, 'https://sta.ncctrials.com/file/avatar/a1002.png', 1, 4, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
 (1003, 'anchor1003', 'La Liga',   '13800138003', '86', '', '', 2, 2, 0, 300, 1, 'https://sta.ncctrials.com/file/avatar/a1003.png', 1, 4, UNIX_TIMESTAMP(), UNIX_TIMESTAMP())
ON DUPLICATE KEY UPDATE `nick_name`=VALUES(`nick_name`);

-- Anchor extra profile
INSERT INTO `anchors` (`uid`,`nick_name`,`icon`,`cut_out_icon`,`intro`,`fans`,`follow`,`hot`,`room_num`,`detail`,`notice`,`live`,`created_at`) VALUES
 (1001, 'Cá Mực FM', 'https://sta.ncctrials.com/file/avatar/a1001.png', '', 'Ngoại hạng Anh mỗi tối', 128000, 98000, 9527, '1001', 'Livestream Ngoại hạng Anh mỗi tối 20:00', 'No spam', 1, UNIX_TIMESTAMP()),
 (1002, 'NBA Boy',   'https://sta.ncctrials.com/file/avatar/a1002.png', '', 'NBA / CBA analyst',       86000,  64000, 7610, '1002', 'NBA stream',                          'Be civil', 1, UNIX_TIMESTAMP()),
 (1003, 'La Liga',   'https://sta.ncctrials.com/file/avatar/a1003.png', '', 'La Liga tactics',         54000,  41000, 5230, '1003', 'La Liga live',                       'Rational talk', 1, UNIX_TIMESTAMP())
ON DUPLICATE KEY UPDATE `nick_name`=VALUES(`nick_name`);

-- Live rooms
INSERT INTO `live_room` (`uid`,`room_num`,`title`,`contact`,`cover`,`custom_cover`,`notice`,`detail`,`live_flv`,`live_m3u8`,`live_status`,`room_status`,`live_type`,`live_type_parent`,`focus_count`,`fictitious_focus_count`,`visit_count`,`fictitious_visit_count`,`mark_type`,`assistant_uid`,`hd`,`stream_type`,`push_content`,`created_at`,`updated_at`) VALUES
 (1001, '1001', 'Ngoại hạng Anh: MU vs Liverpool', 'skype:anchor1001', 'https://sta.ncctrials.com/file/cover/1001.jpg', '', 'No spam', 'MU vs LIV 20:00', '', 'https://live.zbyy.example/1001/hd.m3u8', 1, 1, 0, 1, 12000, 5000, 28000, 8000, 3, 0, 1, 7, 'Welcome to room 1001', UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
 (1002, '1002', 'NBA: Lakers vs Warriors',         'skype:anchor1002', 'https://sta.ncctrials.com/file/cover/1002.jpg', '', 'Be civil', 'LAL vs GSW',       '', 'https://live.zbyy.example/1002/hd.m3u8', 1, 1, 0, 2,  9000, 3000, 41000, 9000, 2, 0, 1, 7, 'Welcome to room 1002', UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
 (1003, '1003', 'La Liga: Real vs Barca',           'skype:anchor1003', 'https://sta.ncctrials.com/file/cover/1003.jpg', '', 'Rational talk', 'RMA vs BAR',   '', '',                                        2, 1, 0, 1,  5000, 1000,  8000, 2000, 1, 0, 0, 7, 'Welcome to room 1003', UNIX_TIMESTAMP(), UNIX_TIMESTAMP())
ON DUPLICATE KEY UPDATE `title`=VALUES(`title`);

-- Sample matches (today + next 6 days). The Go seed tool generates richer data.
INSERT INTO `match_schedule` (`schedule_id`,`host_name`,`guest_name`,`host_score`,`guest_score`,`match_time`,`live_type`,`live_type_parent`,`host_icon`,`guest_icon`,`sub_type_name`,`match_status`,`status`,`hot`,`green_match`,`created_at`) VALUES
 (980754, 'MU',        'Liverpool', 0, 0, NOW(),                       0, 1, 'https://sta.ncctrials.com/file/team/man.png', 'https://sta.ncctrials.com/file/team/liv.png', 'EPL',     NULL, 1, 100, 0, UNIX_TIMESTAMP()),
 (980755, 'Lakers',    'Warriors',  0, 0, NOW() + INTERVAL 1 HOUR,     0, 2, 'https://sta.ncctrials.com/file/team/lal.png', 'https://sta.ncctrials.com/file/team/gsw.png', 'NBA',     NULL, 1,  90, 0, UNIX_TIMESTAMP()),
 (980756, 'Real',      'Barca',     0, 0, NOW() + INTERVAL 2 HOUR,     0, 1, 'https://sta.ncctrials.com/file/team/rma.png', 'https://sta.ncctrials.com/file/team/bar.png', 'La Liga', NULL, 1,  80, 0, UNIX_TIMESTAMP()),
 (980757, 'Analysis',  'Show',      0, 0, NOW() + INTERVAL 3 HOUR,     0, 5, 'https://sta.ncctrials.com/file/team/ana.png', 'https://sta.ncctrials.com/file/team/sho.png', 'Talk',    NULL, 1,  60, 0, UNIX_TIMESTAMP())
ON DUPLICATE KEY UPDATE `host_name`=VALUES(`host_name`);

-- Match ↔ room links
INSERT INTO `match_schedule_room` (`schedule_id`,`room_num`,`status`) VALUES
 (980754, '1001', 1),
 (980755, '1002', 1),
 (980756, '1003', 1),
 (980757, '1001', 1)
ON DUPLICATE KEY UPDATE `status`=VALUES(`status`);

-- Hot recommend
INSERT INTO `live_hot_recommend` (`room_num`,`room_json`,`sort_order`,`status`,`begin_time`,`end_time`) VALUES
 ('1001', '[{"uid":1001,"roomNum":"1001","nickName":"Cá Mực FM","sort":1}]', 1, 1, NOW() - INTERVAL 1 DAY, NOW() + INTERVAL 30 DAY),
 ('1002', '[{"uid":1002,"roomNum":"1002","nickName":"NBA Boy","sort":2}]',   2, 1, NOW() - INTERVAL 1 DAY, NOW() + INTERVAL 30 DAY)
ON DUPLICATE KEY UPDATE `room_json`=VALUES(`room_json`);

-- Sample gift-rank
INSERT INTO `room_gift_rank` (`room_num`,`uid`,`nick_name`,`icon`,`score`,`rank_no`) VALUES
 ('1001', 5001, 'Fan01', 'https://sta.ncctrials.com/file/u/5001.png', 18820, 1),
 ('1001', 5002, 'Fan02', 'https://sta.ncctrials.com/file/u/5002.png', 12330, 2),
 ('1002', 6001, 'Fan03', 'https://sta.ncctrials.com/file/u/6001.png', 22110, 1)
ON DUPLICATE KEY UPDATE `score`=VALUES(`score`);

-- =====================================================================
-- Demo audience user:
--   phone: 13800138888, country_code: 86, plain password: qwe123
--   client sends md5("qwe123") = "200820e3227815ed1756a6b531e7e0d2"
--   salt: "7Whd1U2T1pjeDP4HcSVDxwBMF5Vf6NWx"
--   stored password = md5("200820e3227815ed1756a6b531e7e0d2" + "7Whd1U2T1pjeDP4HcSVDxwBMF5Vf6NWx")
--                  = "8ec733b6de4825a437faee2c01ddd309"
-- =====================================================================
INSERT INTO `user` (`uid`,`login_name`,`nick_name`,`phone`,`country_code`,`password`,`salt`,`pwd_type`,`user_type`,`score`,`grow`,`status`,`icon`,`gender`,`plat`,`created_at`,`updated_at`) VALUES
 (5001, 'demo', 'demo_user', '13800138888', '86', '8ec733b6de4825a437faee2c01ddd309', '7Whd1U2T1pjeDP4HcSVDxwBMF5Vf6NWx', 2, 1, 500, 0, 1, 'https://sta.ncctrials.com/file/avatar/demo.png', 1, 4, UNIX_TIMESTAMP(), UNIX_TIMESTAMP())
ON DUPLICATE KEY UPDATE `password`=VALUES(`password`);
