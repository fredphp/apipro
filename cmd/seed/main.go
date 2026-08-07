package main

// Seed tool — populates the database with zbyy sample data.
// Works with both MySQL and SQLite.
//
// Usage:
//   go run cmd/seed/main.go -config cmd/rpc/etc/apipro.yaml

import (
        "context"
        "database/sql"
        "encoding/json"
        "flag"
        "fmt"
        "os"
        "strings"
        "time"

        "apipro/common/auth"
        "apipro/common/db"

        "github.com/zeromicro/go-zero/core/conf"
        "github.com/zeromicro/go-zero/core/logx"
)

type Config struct {
        DBDriver   string
        DataSource string
}

func main() {
        configFile := flag.String("config", "cmd/rpc/etc/apipro.yaml", "config file")
        flag.Parse()

        var c Config
        conf.MustLoad(*configFile, &c)

        sqlDB, err := db.New(c.DBDriver, c.DataSource)
        if err != nil {
                fmt.Fprintf(os.Stderr, "open db: %v\n", err)
                os.Exit(1)
        }
        defer sqlDB.Close()

        ctx := context.Background()

        if c.DBDriver == "sqlite" || c.DBDriver == "sqlite3" {
                if err := createSqliteTables(ctx, sqlDB); err != nil {
                        fmt.Fprintf(os.Stderr, "create sqlite tables: %v\n", err)
                        os.Exit(1)
                }
        }

        seedAll(ctx, sqlDB, c.DBDriver)
        fmt.Println("✅ seed done")
}

// ---- table creation for SQLite ----

func createSqliteTables(ctx context.Context, db *sql.DB) error {
        stmts := strings.Split(sqliteSchema, ";")
        for _, s := range stmts {
                s = strings.TrimSpace(s)
                if s == "" {
                        continue
                }
                if _, err := db.ExecContext(ctx, s); err != nil {
                        return fmt.Errorf("exec: %w\nSQL: %s", err, s)
                }
        }
        return nil
}

// ---- seed data ----

func seedAll(ctx context.Context, db *sql.DB, driver string) {
        now := time.Now().Unix()

        // live_types
        for _, lt := range liveTypes {
                _, err := db.ExecContext(ctx,
                        `INSERT OR REPLACE INTO live_types (code, name, icon, sort_order, status) VALUES (?,?,?,?,?)`,
                        lt[0], lt[1], lt[2], toInt(lt[3]), 1)
                logx.Must(err)
        }

        // anchors
        for _, a := range anchors {
                _, err := db.ExecContext(ctx,
                        `INSERT OR REPLACE INTO anchors
                        (uid, nick_name, icon, cut_out_icon, intro, fans, follow, hot, room_num, detail, notice, live, created_at)
                        VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
                        a[0], a[1], a[2], a[3], a[4], toInt64(a[5]), toInt64(a[6]), toInt64(a[7]),
                        a[8], a[9], a[10], toInt(a[11]), now)
                logx.Must(err)
        }

        // rooms
        for _, r := range rooms {
                streamJSON, _ := json.Marshal(r[7].([]string))
                tagJSON, _ := json.Marshal(r[9].([]string))
                _, err := db.ExecContext(ctx,
                        `INSERT OR REPLACE INTO rooms
                        (room_num, title, cover, live, view_num, live_type, anchor_uid, stream_urls, notice, tags, cate_name, created_at)
                        VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
                        r[0], r[1], r[2], toInt(r[3]), toInt64(r[4]), r[5], r[6],
                        string(streamJSON), r[8], string(tagJSON), r[10], now)
                logx.Must(err)
        }

        // matches (today + 6 days)
        today := time.Now()
        for d := 0; d < 7; d++ {
                day := today.AddDate(0, 0, d)
                dateKey := day.Format("20060102")
                for _, m := range buildMatchesForDay(day, dateKey) {
                        _, err := db.ExecContext(ctx,
                                `INSERT OR REPLACE INTO matches
                                (schedule_id, sub_cate_name, cate_name, match_time, match_date, host_name, host_icon,
                                 guest_name, guest_icon, venue, status, reservation_status, created_at)
                                VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
                                m...)
                        logx.Must(err)
                }
                // match_anchors
                maLinks := []struct{ MatchID, AnchorUID string }{
                        {dateKey + "01", "A1001"},
                        {dateKey + "01", "A1006"},
                        {dateKey + "02", "A1002"},
                }
                if d%2 == 0 {
                        maLinks = append(maLinks, struct{ MatchID, AnchorUID string }{dateKey + "03", "A1003"})
                }
                if d == 1 {
                        maLinks = append(maLinks, struct{ MatchID, AnchorUID string }{dateKey + "04", "A1006"})
                }
                for _, ma := range maLinks {
                        _, _ = db.ExecContext(ctx,
                                `INSERT OR IGNORE INTO match_anchors (match_id, anchor_uid) VALUES (?,?)`,
                                ma.MatchID, ma.AnchorUID)
                }
        }

        // room_ranks (delete first to avoid duplicates on re-seed)
        _, _ = db.ExecContext(ctx, `DELETE FROM room_ranks`)
        for _, r := range roomRanks {
                _, err := db.ExecContext(ctx,
                        `INSERT INTO room_ranks (room_num, uid, nick_name, icon, score, rank_no) VALUES (?,?,?,?,?,?)`,
                        r[0], r[1], r[2], r[3], toInt64(r[4]), toInt(r[5]))
                logx.Must(err)
        }

        // demo user (password = md5Pwd("123456", 1))
        pwd := auth.Md5Pwd("123456", 1)
        _, err := db.ExecContext(ctx,
                `INSERT OR REPLACE INTO users
                (uid, login_name, nick_name, phone, country_code, password, pwd_type, grow, score, level, avatar, is_user, created_at, updated_at)
                VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
                "U0001", "demo", "demo用户", "13800138000", "+86", pwd, 1, 120, 500, 3,
                "https://cdn.zbyy.example/avatar/demo.png", 1, now, now)
        logx.Must(err)
}

func buildMatchesForDay(day time.Time, dateKey string) [][]interface{} {
        host := func(t time.Time) string {
                now := time.Now()
                if now.Before(t) {
                        return "not_started"
                }
                if now.Sub(t) < 2*time.Hour+30*time.Minute {
                        return "living"
                }
                return "over"
        }
        return [][]interface{}{
                {dateKey + "01", "英超", "足球", day.Add(20 * time.Hour).Format("2006-01-02 15:04:05"), dateKey,
                        "曼联", "https://cdn.zbyy.example/team/man.png",
                        "利物浦", "https://cdn.zbyy.example/team/liv.png",
                        "老特拉福德", host(day.Add(20 * time.Hour)), 0, time.Now().Unix()},
                {dateKey + "02", "NBA", "篮球", day.Add(11 * time.Hour).Format("2006-01-02 15:04:05"), dateKey,
                        "湖人", "https://cdn.zbyy.example/team/lal.png",
                        "勇士", "https://cdn.zbyy.example/team/gsw.png",
                        "Crypto.com Arena", host(day.Add(11 * time.Hour)), 0, time.Now().Unix()},
                {dateKey + "03", "西甲", "足球", day.Add(23 * time.Hour).Format("2006-01-02 15:04:05"), dateKey,
                        "皇马", "https://cdn.zbyy.example/team/rma.png",
                        "巴萨", "https://cdn.zbyy.example/team/bar.png",
                        "伯纳乌", host(day.Add(23 * time.Hour)), 0, time.Now().Unix()},
        }
}

// ---- helpers ----

func toInt(v interface{}) int {
        switch t := v.(type) {
        case int:
                return t
        case int32:
                return int(t)
        case int64:
                return int(t)
        case string:
                i := 0
                fmt.Sscanf(t, "%d", &i)
                return i
        }
        return 0
}

func toInt64(v interface{}) int64 {
        return int64(toInt(v))
}
