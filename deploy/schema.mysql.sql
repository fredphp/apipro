-- =====================================================================
-- apipro MySQL schema (production)
-- Derived from the zbyy live-streaming platform data model.
-- Run:  mysql -u root -p < deploy/schema.mysql.sql
-- =====================================================================

CREATE DATABASE IF NOT EXISTS apipro DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE apipro;

-- ---------------------------------------------------------------------
-- Users (注册/登录)
--   password stores the CLIENT-encrypted value (zbyy md5 algorithm):
--     pwdType=1: md5( md5(password.toLowerCase()) + '&%*$8@!!%' )
--     pwdType=2: md5(password)
--   The server stores/compares what the client sends — never the raw pw.
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `users` (
  `uid`          VARCHAR(32)  NOT NULL COMMENT 'unique user id',
  `login_name`   VARCHAR(64)  NOT NULL DEFAULT '',
  `nick_name`    VARCHAR(64)  NOT NULL DEFAULT '',
  `phone`        VARCHAR(20)  NOT NULL DEFAULT '',
  `country_code` VARCHAR(8)   NOT NULL DEFAULT '',
  `password`     VARCHAR(64)  NOT NULL DEFAULT '' COMMENT 'client md5 hash',
  `pwd_type`     TINYINT      NOT NULL DEFAULT 1 COMMENT '1=md5(md5+secret), 2=md5',
  `grow`         BIGINT       NOT NULL DEFAULT 0,
  `score`        BIGINT       NOT NULL DEFAULT 0,
  `level`        INT          NOT NULL DEFAULT 1,
  `avatar`       VARCHAR(255) NOT NULL DEFAULT '',
  `is_user`      TINYINT      NOT NULL DEFAULT 1 COMMENT '1=registered, 0=guest',
  `created_at`   BIGINT       NOT NULL DEFAULT 0,
  `updated_at`   BIGINT       NOT NULL DEFAULT 0,
  PRIMARY KEY (`uid`),
  UNIQUE KEY `uk_phone` (`country_code`, `phone`),
  UNIQUE KEY `uk_loginname` (`login_name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='users';

-- ---------------------------------------------------------------------
-- Live types (直播分类)
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `live_types` (
  `code`       VARCHAR(32)  NOT NULL,
  `name`       VARCHAR(64)  NOT NULL DEFAULT '',
  `icon`       VARCHAR(255) NOT NULL DEFAULT '',
  `sort_order` INT          NOT NULL DEFAULT 0,
  `status`     TINYINT      NOT NULL DEFAULT 1,
  PRIMARY KEY (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='live types';

-- ---------------------------------------------------------------------
-- Anchors / Commentators (解说员)
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `anchors` (
  `uid`          VARCHAR(32)  NOT NULL,
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='anchors / commentators';

-- ---------------------------------------------------------------------
-- Rooms (直播间)
--   stream_urls / tags stored as JSON text.
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `rooms` (
  `room_num`   VARCHAR(16)  NOT NULL,
  `title`      VARCHAR(200) NOT NULL DEFAULT '',
  `cover`      VARCHAR(255) NOT NULL DEFAULT '',
  `live`       TINYINT      NOT NULL DEFAULT 0,
  `view_num`   BIGINT       NOT NULL DEFAULT 0,
  `live_type`  VARCHAR(32)  NOT NULL DEFAULT '',
  `anchor_uid` VARCHAR(32)  NOT NULL DEFAULT '',
  `stream_urls` TEXT        COMMENT 'JSON array of url strings',
  `notice`     VARCHAR(500) NOT NULL DEFAULT '',
  `tags`       TEXT         COMMENT 'JSON array of tag strings',
  `cate_name`  VARCHAR(64)  NOT NULL DEFAULT '',
  `created_at` BIGINT       NOT NULL DEFAULT 0,
  PRIMARY KEY (`room_num`),
  KEY `idx_anchor` (`anchor_uid`),
  KEY `idx_live` (`live`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='rooms';

-- ---------------------------------------------------------------------
-- Matches (赛程/比赛)
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `matches` (
  `schedule_id`        VARCHAR(32)  NOT NULL,
  `sub_cate_name`      VARCHAR(64)  NOT NULL DEFAULT '',
  `cate_name`          VARCHAR(64)  NOT NULL DEFAULT '',
  `match_time`         VARCHAR(32)  NOT NULL DEFAULT '' COMMENT 'YYYY-MM-DD HH:MM:SS',
  `match_date`         VARCHAR(8)   NOT NULL DEFAULT '' COMMENT 'YYYYMMDD for fast date lookup',
  `host_name`          VARCHAR(64)  NOT NULL DEFAULT '',
  `host_icon`          VARCHAR(255) NOT NULL DEFAULT '',
  `guest_name`         VARCHAR(64)  NOT NULL DEFAULT '',
  `guest_icon`         VARCHAR(255) NOT NULL DEFAULT '',
  `venue`              VARCHAR(128) NOT NULL DEFAULT '',
  `status`             VARCHAR(16)  NOT NULL DEFAULT 'not_started' COMMENT 'not_started|living|over',
  `reservation_status` INT          NOT NULL DEFAULT 0,
  `created_at`         BIGINT       NOT NULL DEFAULT 0,
  PRIMARY KEY (`schedule_id`),
  KEY `idx_date` (`match_date`),
  KEY `idx_cate` (`cate_name`),
  KEY `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='matches';

-- ---------------------------------------------------------------------
-- Match ↔ Anchor (many-to-many: a match can have multiple commentators)
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `match_anchors` (
  `match_id`  VARCHAR(32) NOT NULL,
  `anchor_uid` VARCHAR(32) NOT NULL,
  PRIMARY KEY (`match_id`, `anchor_uid`),
  KEY `idx_anchor` (`anchor_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='match-anchor relation';

-- ---------------------------------------------------------------------
-- Room rank (排行榜)
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `room_ranks` (
  `id`        BIGINT       NOT NULL AUTO_INCREMENT,
  `room_num`  VARCHAR(16)  NOT NULL,
  `uid`       VARCHAR(32)  NOT NULL DEFAULT '',
  `nick_name` VARCHAR(64)  NOT NULL DEFAULT '',
  `icon`      VARCHAR(255) NOT NULL DEFAULT '',
  `score`     BIGINT       NOT NULL DEFAULT 0,
  `rank_no`   INT          NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_room` (`room_num`, `rank_no`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='room ranks';

-- =====================================================================
-- Seed data
-- =====================================================================

INSERT INTO `live_types` (`code`,`name`,`icon`,`sort_order`,`status`) VALUES
 ('football','足球','https://cdn.zbyy.example/ico/football.png',1,1),
 ('basketball','篮球','https://cdn.zbyy.example/ico/basketball.png',2,1),
 ('snooker','斯诺克','https://cdn.zbyy.example/ico/snooker.png',3,1),
 ('other','其它','https://cdn.zbyy.example/ico/other.png',4,1)
ON DUPLICATE KEY UPDATE `name`=VALUES(`name`);

INSERT INTO `anchors` (`uid`,`nick_name`,`icon`,`cut_out_icon`,`intro`,`fans`,`follow`,`hot`,`room_num`,`detail`,`notice`,`live`,`created_at`) VALUES
 ('A1001','飞鱼解说','https://cdn.zbyy.example/avatar/a1001.png','https://cdn.zbyy.example/avatar/a1001_cut.png','前职业球员，专注英超解说10年',128000,98000,9527,'1001','每晚8点英超直播','禁止刷屏、禁止广告',1,UNIX_TIMESTAMP()),
 ('A1002','篮球小子','https://cdn.zbyy.example/avatar/a1002.png','https://cdn.zbyy.example/avatar/a1002_cut.png','NBA/CBA深度分析',86000,64000,7610,'1002','NBA专场','文明观赛',1,UNIX_TIMESTAMP()),
 ('A1003','绿茵观察','https://cdn.zbyy.example/avatar/a1003.png','https://cdn.zbyy.example/avatar/a1003_cut.png','西甲、欧冠战术分析',54000,41000,5230,'1003','西甲之夜','理性讨论',0,UNIX_TIMESTAMP()),
 ('A1004','斯诺克达人','https://cdn.zbyy.example/avatar/a1004.png','https://cdn.zbyy.example/avatar/a1004_cut.png','斯诺克职业赛事解说',21000,18000,2100,'1004','斯诺克直播','安静观赛',0,UNIX_TIMESTAMP()),
 ('A1005','中超前线','https://cdn.zbyy.example/avatar/a1005.png','https://cdn.zbyy.example/avatar/a1005_cut.png','中超、亚冠现场报道',39000,30000,3340,'1005','中超集锦','禁止地域攻击',1,UNIX_TIMESTAMP()),
 ('A1006','德甲工匠','https://cdn.zbyy.example/avatar/a1006.png','https://cdn.zbyy.example/avatar/a1006_cut.png','德甲战术拆解',47000,35000,4120,'1006','德甲周末','文明互动',0,UNIX_TIMESTAMP())
ON DUPLICATE KEY UPDATE `nick_name`=VALUES(`nick_name`);

INSERT INTO `rooms` (`room_num`,`title`,`cover`,`live`,`view_num`,`live_type`,`anchor_uid`,`stream_urls`,`notice`,`tags`,`cate_name`,`created_at`) VALUES
 ('1001','英超焦点战: 曼联 vs 利物浦','https://cdn.zbyy.example/cover/1001.jpg',1,38211,'football','A1001','["https://live.zbyy.example/1001/hd.m3u8","https://live.zbyy.example/1001/sd.m3u8"]','文明观赛，禁止刷屏','["英超","曼联","利物浦"]','英超',UNIX_TIMESTAMP()),
 ('1002','NBA常规赛: 湖人 vs 勇士','https://cdn.zbyy.example/cover/1002.jpg',1,51209,'basketball','A1002','["https://live.zbyy.example/1002/hd.m3u8"]','理性讨论','["NBA","湖人","勇士"]','NBA',UNIX_TIMESTAMP()),
 ('1003','西甲: 皇马 vs 巴萨','https://cdn.zbyy.example/cover/1003.jpg',0,0,'football','A1003','[]','比赛尚未开始','["西甲","国家德比"]','西甲',UNIX_TIMESTAMP()),
 ('1004','斯诺克世锦赛 半决赛','https://cdn.zbyy.example/cover/1004.jpg',0,0,'snooker','A1004','[]','静音观赛','["斯诺克","世锦赛"]','斯诺克',UNIX_TIMESTAMP()),
 ('1005','中超第20轮: 海港 vs 申花','https://cdn.zbyy.example/cover/1005.jpg',1,19887,'football','A1005','["https://live.zbyy.example/1005/hd.m3u8"]','禁止地域攻击','["中超","上海德比"]','中超',UNIX_TIMESTAMP()),
 ('1006','德甲: 拜仁 vs 多特','https://cdn.zbyy.example/cover/1006.jpg',0,0,'football','A1006','[]','德国国家德比','["德甲","拜仁","多特"]','德甲',UNIX_TIMESTAMP())
ON DUPLICATE KEY UPDATE `title`=VALUES(`title`);

-- Matches: today + next 6 days. Uses a procedure to generate date-aware seed.
-- For simplicity we insert static seed for today; the application also has a
-- seed helper that can populate future dates. Run the Go seed tool if needed:
--   go run cmd/seed/main.go
INSERT INTO `room_ranks` (`room_num`,`uid`,`nick_name`,`icon`,`score`,`rank_no`) VALUES
 ('1001','U5001','球迷老王','https://cdn.zbyy.example/u/5001.png',18820,1),
 ('1001','U5002','红魔死忠','https://cdn.zbyy.example/u/5002.png',12330,2),
 ('1001','U5003','安菲尔德之心','https://cdn.zbyy.example/u/5003.png',9910,3),
 ('1002','U6001','紫金王朝','https://cdn.zbyy.example/u/6001.png',22110,1),
 ('1002','U6002','萌神粉丝','https://cdn.zbyy.example/u/6002.png',15020,2)
ON DUPLICATE KEY UPDATE `score`=VALUES(`score`);

-- A demo user (password = md5Pwd("123456", 1) computed client-side):
--   md5("123456") = e10adc3949ba59abbe56e057f20f883e
--   md5(e10adc3949ba59abbe56e057f20f883e + "&%*$8@!!%") = 2e36f5fa46a866a6e91b71524dd8d155
INSERT INTO `users` (`uid`,`login_name`,`nick_name`,`phone`,`country_code`,`password`,`pwd_type`,`grow`,`score`,`level`,`avatar`,`is_user`,`created_at`,`updated_at`) VALUES
 ('U0001','demo','demo用户','13800138000','+86','2e36f5fa46a866a6e91b71524dd8d155',1,120,500,3,'https://cdn.zbyy.example/avatar/demo.png',1,UNIX_TIMESTAMP(),UNIX_TIMESTAMP())
ON DUPLICATE KEY UPDATE `nick_name`=VALUES(`nick_name`);
