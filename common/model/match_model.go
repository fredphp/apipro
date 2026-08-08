package model

// MatchSchedule model — reads from `match_schedule` + `match_schedule_room`
// + `live_room` + `user` (anchor).
//
// MatchCatalogRow is the joined result used to build MatchCatalogItem JSON.

import (
        "context"
        "database/sql"
        "fmt"
        "time"
)

type MatchSchedule struct {
        ScheduleID     int64
        HostName       string
        GuestName      string
        HostScore      sql.NullInt64
        GuestScore     sql.NullInt64
        MatchTime      sql.NullTime
        LiveType       sql.NullInt64
        LiveTypeParent sql.NullInt64
        HostIcon       string
        GuestIcon      string
        SubCateName    string
        MatchStatus    sql.NullInt64
        Status         int32
        Hot            int32
        GreenMatch     int32
}

// MatchCatalogRow is the joined row used for the catalog JSON.
type MatchCatalogRow struct {
        ScheduleID     int64
        MatchTime      string // DATETIME returned as string (SQLite has no DATETIME type)
        HostName       string
        GuestName      string
        HostScore      int
        GuestScore     int
        MatchStatus    int
        HostIcon       string
        GuestIcon      string
        SubCateName    string
        LiveType       int64
        LiveTypeParent int64
        CategoryIcon   string // lt.icon (live_type join on live_type_parent)
        CategoryName   string // lt.type_name
        AnchorUID      int64
        AnchorNickName string
        AnchorIcon     string
        RoomNum        string
        RoomDetail     string
        RoomNotice     string
}

type MatchModel struct {
        db *sql.DB
}

func NewMatchModel(db *sql.DB) *MatchModel { return &MatchModel{db: db} }

// ListCatalog returns the joined catalog rows for live_type_parent ∈ parents,
// enabled status, ordered by match_time ASC. Each row is one (match, anchor)
// pair — the caller must group by schedule_id.
func (m *MatchModel) ListCatalog(ctx context.Context, parents []int64, limit int) ([]MatchCatalogRow, error) {
        if limit <= 0 {
                limit = 100
        }
        if len(parents) == 0 {
                return nil, nil
        }
        // Build IN clause
        args := []interface{}{}
        q := `SELECT ms.schedule_id, ms.match_time, ms.host_name, ms.guest_name,
                       COALESCE(ms.host_score, 0), COALESCE(ms.guest_score, 0),
                       COALESCE(ms.match_status, 0), ms.host_icon, ms.guest_icon,
                       ms.sub_type_name, COALESCE(ms.live_type, 0), COALESCE(ms.live_type_parent, 0),
                       COALESCE(lt.icon, ''), COALESCE(lt.type_name, ''),
                       u.uid, u.nick_name, u.icon,
                       r.room_num, r.detail, r.notice
                FROM match_schedule ms
                INNER JOIN match_schedule_room msr ON msr.schedule_id = ms.schedule_id AND msr.status = 1
                LEFT JOIN live_room r ON r.room_num = msr.room_num AND r.room_status = 1
                LEFT JOIN user u ON u.uid = r.uid
                LEFT JOIN live_type lt ON lt.live_type_id = ms.live_type_parent
                WHERE ms.status = 1 AND ms.live_type_parent IN (` + placeholders(len(parents)) + `)
                ORDER BY ms.match_time ASC LIMIT ?`
        for _, p := range parents {
                args = append(args, p)
        }
        args = append(args, limit)
        rows, err := m.db.QueryContext(ctx, q, args...)
        if err != nil {
                return nil, err
        }
        defer rows.Close()
        return scanCatalogRows(rows)
}

// ListByDate returns catalog rows for a specific date (YYYYMMDD).
func (m *MatchModel) ListByDate(ctx context.Context, dateStr string, limit int) ([]MatchCatalogRow, error) {
        if limit <= 0 {
                limit = 100
        }
        // dateStr is YYYYMMDD; convert to YYYY-MM-DD for the SQL compare (stored
        // as "YYYY-MM-DD HH:MM:SS" text in SQLite / DATETIME in MySQL).
        // Accept either YYYYMMDD or YYYY-MM-DD input.
        normalized := dateStr
        if len(dateStr) == 8 {
                normalized = dateStr[:4] + "-" + dateStr[4:6] + "-" + dateStr[6:8]
        }
        start := normalized + " 00:00:00"
        end := normalized + " 23:59:59"
        rows, err := m.db.QueryContext(ctx, `
                SELECT ms.schedule_id, ms.match_time, ms.host_name, ms.guest_name,
                       COALESCE(ms.host_score, 0), COALESCE(ms.guest_score, 0),
                       COALESCE(ms.match_status, 0), ms.host_icon, ms.guest_icon,
                       ms.sub_type_name, COALESCE(ms.live_type, 0), COALESCE(ms.live_type_parent, 0),
                       COALESCE(lt.icon, ''), COALESCE(lt.type_name, ''),
                       u.uid, u.nick_name, u.icon,
                       r.room_num, r.detail, r.notice
                FROM match_schedule ms
                INNER JOIN match_schedule_room msr ON msr.schedule_id = ms.schedule_id AND msr.status = 1
                LEFT JOIN live_room r ON r.room_num = msr.room_num AND r.room_status = 1
                LEFT JOIN user u ON u.uid = r.uid
                LEFT JOIN live_type lt ON lt.live_type_id = ms.live_type_parent
                WHERE ms.status = 1 AND ms.match_time >= ? AND ms.match_time <= ?
                ORDER BY ms.match_time ASC LIMIT ?`, start, end, limit)
        if err != nil {
                return nil, err
        }
        defer rows.Close()
        return scanCatalogRows(rows)
}

// ListRecommend returns up to N hot upcoming matches (with anchors). Requires
// at least one linked anchor room (EXISTS in match_schedule_room).
func (m *MatchModel) ListRecommend(ctx context.Context, limit int) ([]MatchCatalogRow, error) {
        if limit <= 0 {
                limit = 8
        }
        rows, err := m.db.QueryContext(ctx, `
                SELECT ms.schedule_id, ms.match_time, ms.host_name, ms.guest_name,
                       COALESCE(ms.host_score, 0), COALESCE(ms.guest_score, 0),
                       COALESCE(ms.match_status, 0), ms.host_icon, ms.guest_icon,
                       ms.sub_type_name, COALESCE(ms.live_type, 0), COALESCE(ms.live_type_parent, 0),
                       COALESCE(lt.icon, ''), COALESCE(lt.type_name, ''),
                       u.uid, u.nick_name, u.icon,
                       r.room_num, r.detail, r.notice
                FROM match_schedule ms
                INNER JOIN match_schedule_room msr ON msr.schedule_id = ms.schedule_id AND msr.status = 1
                LEFT JOIN live_room r ON r.room_num = msr.room_num AND r.room_status = 1
                LEFT JOIN user u ON u.uid = r.uid
                LEFT JOIN live_type lt ON lt.live_type_id = ms.live_type_parent
                WHERE ms.status = 1
                ORDER BY ms.hot DESC, ms.match_time ASC LIMIT ?`, limit)
        if err != nil {
                return nil, err
        }
        defer rows.Close()
        return scanCatalogRows(rows)
}

// ListByRoom returns schedules linked to a room (for /room/:roomNum/schedule.json).
func (m *MatchModel) ListByRoom(ctx context.Context, roomNum string, limit int) ([]MatchCatalogRow, error) {
        if limit <= 0 {
                limit = 50
        }
        rows, err := m.db.QueryContext(ctx, `
                SELECT ms.schedule_id, ms.match_time, ms.host_name, ms.guest_name,
                       COALESCE(ms.host_score, 0), COALESCE(ms.guest_score, 0),
                       COALESCE(ms.match_status, 0), ms.host_icon, ms.guest_icon,
                       ms.sub_type_name, COALESCE(ms.live_type, 0), COALESCE(ms.live_type_parent, 0),
                       COALESCE(lt.icon, ''), COALESCE(lt.type_name, ''),
                       u.uid, u.nick_name, u.icon,
                       r.room_num, r.detail, r.notice
                FROM match_schedule ms
                INNER JOIN match_schedule_room msr ON msr.schedule_id = ms.schedule_id AND msr.status = 1 AND msr.room_num = ?
                LEFT JOIN live_room r ON r.room_num = msr.room_num AND r.room_status = 1
                LEFT JOIN user u ON u.uid = r.uid
                LEFT JOIN live_type lt ON lt.live_type_id = ms.live_type_parent
                WHERE ms.status = 1
                ORDER BY ms.match_time ASC LIMIT ?`, roomNum, limit)
        if err != nil {
                return nil, err
        }
        defer rows.Close()
        return scanCatalogRows(rows)
}

// ListRoomsBySchedule returns the live rooms linked to a schedule (for
// /match/detail "rooms" field). Mirrors ListByType/ListHot column shape so
// scanLiveRooms works unchanged.
func (m *MatchModel) ListRoomsBySchedule(ctx context.Context, scheduleID int64, limit int) ([]LiveRoom, error) {
        if limit <= 0 {
                limit = 50
        }
        rows, err := m.db.QueryContext(ctx, `
                SELECT `+liveRoomCols+`,
                       u.nick_name, u.icon,
                       u2.nick_name, u2.icon
                FROM match_schedule_room msr
                INNER JOIN live_room r ON r.room_num = msr.room_num AND r.room_status = 1
                LEFT JOIN user u  ON u.uid  = r.uid
                LEFT JOIN user u2 ON u2.uid = r.assistant_uid
                WHERE msr.schedule_id = ? AND msr.status = 1
                ORDER BY r.mark_type DESC, (r.visit_count + r.fictitious_visit_count) DESC, r.uid ASC
                LIMIT ?`, scheduleID, limit)
        if err != nil {
                return nil, err
        }
        defer rows.Close()
        return scanLiveRooms(rows)
}

func scanCatalogRows(rows *sql.Rows) ([]MatchCatalogRow, error) {
        var out []MatchCatalogRow
        for rows.Next() {
                var r MatchCatalogRow
                var anchorUID sql.NullInt64
                var anchorNick, anchorIcon, roomNum, roomDetail, roomNotice sql.NullString
                if err := rows.Scan(&r.ScheduleID, &r.MatchTime, &r.HostName, &r.GuestName,
                        &r.HostScore, &r.GuestScore, &r.MatchStatus, &r.HostIcon, &r.GuestIcon,
                        &r.SubCateName, &r.LiveType, &r.LiveTypeParent,
                        &r.CategoryIcon, &r.CategoryName,
                        &anchorUID, &anchorNick, &anchorIcon,
                        &roomNum, &roomDetail, &roomNotice); err != nil {
                        return nil, err
                }
                r.AnchorUID = anchorUID.Int64
                r.AnchorNickName = anchorNick.String
                r.AnchorIcon = anchorIcon.String
                r.RoomNum = roomNum.String
                r.RoomDetail = roomDetail.String
                r.RoomNotice = roomNotice.String
                out = append(out, r)
        }
        return out, rows.Err()
}

func placeholders(n int) string {
        if n <= 0 {
                return ""
        }
        out := "?"
        for i := 1; i < n; i++ {
                out += ",?"
        }
        return out
}

// MatchTimeToMS converts a match_time string (e.g. "2026-08-08 05:02:28")
// to milliseconds since epoch (UTC). Returns 0 if empty or invalid.
// Handles both Go-style and SQLite strftime() output.
func MatchTimeToMS(s string) int64 {
        if s == "" {
                return 0
        }
        for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", time.RFC3339, "2006-01-02"} {
                if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
                        return t.UTC().UnixMilli()
                }
        }
        return 0
}

// FormatDateKey returns YYYYMMDD for a time.
func FormatDateKey(t time.Time) string {
        return t.Format("20060102")
}

// DebugString for MatchSchedule (logging).
func (ms *MatchSchedule) DebugString() string {
        return fmt.Sprintf("schedule=%d %s vs %s parent=%d", ms.ScheduleID, ms.HostName, ms.GuestName, ms.LiveTypeParent.Int64)
}
