package model

// Match model — reads from the `matches` table + `match_anchors` join.

import (
        "context"
        "database/sql"
        "fmt"
)

type Match struct {
        ScheduleId        string
        SubCateName       string
        CateName          string
        MatchTime         string
        MatchDate         string
        HostName          string
        HostIcon          string
        GuestName         string
        GuestIcon         string
        Venue             string
        Status            string
        ReservationStatus int32
        CreatedAt         int64
}

type MatchModel struct {
        db *sql.DB
}

func NewMatchModel(db *sql.DB) *MatchModel { return &MatchModel{db: db} }

const matchCols = `schedule_id, sub_cate_name, cate_name, match_time, match_date, host_name, host_icon, guest_name, guest_icon, venue, status, reservation_status, created_at`

func (m *MatchModel) FindByID(ctx context.Context, id string) (*Match, error) {
        row := m.db.QueryRowContext(ctx, `SELECT `+matchCols+` FROM matches WHERE schedule_id=?`, id)
        return scanMatch(row)
}

func (m *MatchModel) ListByDate(ctx context.Context, date string) ([]Match, error) {
        rows, err := m.db.QueryContext(ctx, `SELECT `+matchCols+` FROM matches WHERE match_date=? ORDER BY match_time ASC`, date)
        if err != nil {
                return nil, err
        }
        defer rows.Close()
        return scanMatches(rows)
}

func (m *MatchModel) ListByCate(ctx context.Context, cate string) ([]Match, error) {
        rows, err := m.db.QueryContext(ctx, `SELECT `+matchCols+` FROM matches WHERE cate_name=? ORDER BY match_time ASC`, cate)
        if err != nil {
                return nil, err
        }
        defer rows.Close()
        return scanMatches(rows)
}

func (m *MatchModel) ListRecommend(ctx context.Context, limit int) ([]Match, error) {
        if limit <= 0 {
                limit = 5
        }
        rows, err := m.db.QueryContext(ctx,
                `SELECT `+matchCols+` FROM matches WHERE status IN ('living','not_started') ORDER BY status ASC, match_time ASC LIMIT ?`, limit)
        if err != nil {
                return nil, err
        }
        defer rows.Close()
        return scanMatches(rows)
}

func (m *MatchModel) ListCateNames(ctx context.Context) ([]string, error) {
        rows, err := m.db.QueryContext(ctx, `SELECT DISTINCT cate_name FROM matches WHERE cate_name != '' ORDER BY cate_name ASC`)
        if err != nil {
                return nil, err
        }
        defer rows.Close()
        var out []string
        for rows.Next() {
                var s string
                if err := rows.Scan(&s); err != nil {
                        return nil, err
                }
                out = append(out, s)
        }
        return out, rows.Err()
}

// ListByAnchorRoom returns matches whose linked anchors own the given room.
func (m *MatchModel) ListByAnchorRoom(ctx context.Context, roomNum string) ([]Match, error) {
        rows, err := m.db.QueryContext(ctx,
                `SELECT DISTINCT m.schedule_id, m.sub_cate_name, m.cate_name, m.match_time,
                 m.match_date, m.host_name, m.host_icon, m.guest_name, m.guest_icon,
                 m.venue, m.status, m.reservation_status, m.created_at
                 FROM matches m
                 INNER JOIN match_anchors ma ON ma.match_id = m.schedule_id
                 INNER JOIN anchors a ON a.uid = ma.anchor_uid
                 WHERE a.room_num = ?
                 ORDER BY m.match_time ASC`, roomNum)
        if err != nil {
                return nil, err
        }
        defer rows.Close()
        return scanMatches(rows)
}

func scanMatch(row *sql.Row) (*Match, error) {
        var mm Match
        err := row.Scan(&mm.ScheduleId, &mm.SubCateName, &mm.CateName, &mm.MatchTime,
                &mm.MatchDate, &mm.HostName, &mm.HostIcon, &mm.GuestName, &mm.GuestIcon,
                &mm.Venue, &mm.Status, &mm.ReservationStatus, &mm.CreatedAt)
        if err != nil {
                if err == sql.ErrNoRows {
                        return nil, ErrNotFound
                }
                return nil, err
        }
        return &mm, nil
}

func scanMatches(rows *sql.Rows) ([]Match, error) {
        var out []Match
        for rows.Next() {
                var mm Match
                if err := rows.Scan(&mm.ScheduleId, &mm.SubCateName, &mm.CateName, &mm.MatchTime,
                        &mm.MatchDate, &mm.HostName, &mm.HostIcon, &mm.GuestName, &mm.GuestIcon,
                        &mm.Venue, &mm.Status, &mm.ReservationStatus, &mm.CreatedAt); err != nil {
                        return nil, err
                }
                out = append(out, mm)
        }
        return out, rows.Err()
}

func (m *Match) DebugString() string {
        return fmt.Sprintf("id=%s %s vs %s %s", m.ScheduleId, m.HostName, m.GuestName, m.Status)
}

// LiveType model
type LiveType struct {
        Code      string
        Name      string
        Icon      string
        SortOrder int32
}

func (m *MatchModel) ListLiveTypes(ctx context.Context) ([]LiveType, error) {
        rows, err := m.db.QueryContext(ctx,
                `SELECT code, name, icon, sort_order FROM live_types WHERE status=1 ORDER BY sort_order ASC`)
        if err != nil {
                return nil, err
        }
        defer rows.Close()
        var out []LiveType
        for rows.Next() {
                var lt LiveType
                if err := rows.Scan(&lt.Code, &lt.Name, &lt.Icon, &lt.SortOrder); err != nil {
                        return nil, err
                }
                out = append(out, lt)
        }
        return out, rows.Err()
}
