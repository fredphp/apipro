package model

// LiveRoom model — reads from `live_room` table joined with `user` (anchor) +
// `user` (assistant). Mirrors backend-zero LiveRoom read model.

import (
        "context"
        "database/sql"
        "fmt"
)

type LiveRoom struct {
        UID                  int64  // r.uid (anchor's user id)
        RoomNum              string // r.room_num
        Title                string
        Contact              string
        Cover                string
        CustomCover          string
        Notice               string
        Detail               string
        LiveFLV              string
        LiveM3U8             string
        LiveStatus           int    // 1=on-air, 2=off
        RoomStatus           int    // 1=visible
        LiveType             int64  // child id
        LiveTypeParent       int64  // 1=football, 2=basketball, 5=analysis
        FocusCount           int64
        FictitiousFocusCount int64
        VisitCount           int64
        FictitiousVisitCount int64
        MarkType             int
        AssistantUID         int64
        HD                   int
        StreamType           int
        PushContent          string
        AnchorNickName       string // joined from user (anchor)
        AnchorIcon           string
        AssistantNickName    string // joined from user (assistant)
        AssistantIcon        string
}

type LiveRoomModel struct {
        db *sql.DB
}

func NewLiveRoomModel(db *sql.DB) *LiveRoomModel { return &LiveRoomModel{db: db} }

const liveRoomCols = `r.uid, r.room_num, r.title, r.contact, r.cover, r.custom_cover, r.notice, r.detail,
        r.live_flv, r.live_m3u8, r.live_status, r.room_status, r.live_type, r.live_type_parent,
        r.focus_count, r.fictitious_focus_count, r.visit_count, r.fictitious_visit_count,
        r.mark_type, r.assistant_uid, r.hd, r.stream_type, r.push_content`

// FindByRoomNum returns one room joined with anchor + assistant user info.
func (m *LiveRoomModel) FindByRoomNum(ctx context.Context, roomNum string) (*LiveRoom, error) {
        row := m.db.QueryRowContext(ctx, `
                SELECT `+liveRoomCols+`,
                       u.nick_name, u.icon,
                       u2.nick_name, u2.icon
                FROM `+Tbl("live_room")+` r
                LEFT JOIN `+Tbl("user")+` u  ON u.uid  = r.uid
                LEFT JOIN `+Tbl("user")+` u2 ON u2.uid = r.assistant_uid
                WHERE r.room_num=?`, roomNum)
        return scanLiveRoom(row)
}

// ListHot returns live rooms filtered live_status=1 AND room_status=1, ordered
// by mark_type DESC, (visit_count + fictitious_visit_count) DESC, uid ASC.
func (m *LiveRoomModel) ListHot(ctx context.Context, limit int) ([]LiveRoom, error) {
        if limit <= 0 {
                limit = 50
        }
        rows, err := m.db.QueryContext(ctx, `
                SELECT `+liveRoomCols+`,
                       u.nick_name, u.icon,
                       u2.nick_name, u2.icon
                FROM `+Tbl("live_room")+` r
                LEFT JOIN `+Tbl("user")+` u  ON u.uid  = r.uid
                LEFT JOIN `+Tbl("user")+` u2 ON u2.uid = r.assistant_uid
                WHERE r.live_status=1 AND r.room_status=1
                ORDER BY r.mark_type DESC, (r.visit_count + r.fictitious_visit_count) DESC, r.uid ASC
                LIMIT ?`, limit)
        if err != nil {
                return nil, err
        }
        defer rows.Close()
        return scanLiveRooms(rows)
}

// ListByType returns rooms by top-level live_type_parent.
func (m *LiveRoomModel) ListByType(ctx context.Context, parentID int64, limit int) ([]LiveRoom, error) {
        if limit <= 0 {
                limit = 50
        }
        rows, err := m.db.QueryContext(ctx, `
                SELECT `+liveRoomCols+`,
                       u.nick_name, u.icon,
                       u2.nick_name, u2.icon
                FROM `+Tbl("live_room")+` r
                LEFT JOIN `+Tbl("user")+` u  ON u.uid  = r.uid
                LEFT JOIN `+Tbl("user")+` u2 ON u2.uid = r.assistant_uid
                WHERE r.room_status=1 AND r.live_type_parent=?
                ORDER BY r.mark_type DESC, (r.visit_count + r.fictitious_visit_count) DESC, r.uid ASC
                LIMIT ?`, parentID, limit)
        if err != nil {
                return nil, err
        }
        defer rows.Close()
        return scanLiveRooms(rows)
}

// ListAllVisible returns all visible rooms (room_status=1) — used for the "0"
// all-merged group in all_live_rooms.json.
func (m *LiveRoomModel) ListAllVisible(ctx context.Context, limit int) ([]LiveRoom, error) {
        if limit <= 0 {
                limit = 200
        }
        rows, err := m.db.QueryContext(ctx, `
                SELECT `+liveRoomCols+`,
                       u.nick_name, u.icon,
                       u2.nick_name, u2.icon
                FROM `+Tbl("live_room")+` r
                LEFT JOIN `+Tbl("user")+` u  ON u.uid  = r.uid
                LEFT JOIN `+Tbl("user")+` u2 ON u2.uid = r.assistant_uid
                WHERE r.room_status=1
                ORDER BY r.mark_type DESC, (r.visit_count + r.fictitious_visit_count) DESC, r.uid ASC
                LIMIT ?`, limit)
        if err != nil {
                return nil, err
        }
        defer rows.Close()
        return scanLiveRooms(rows)
}

// BumpVisitCount atomically increments visit_count (used by WS room enter).
func (m *LiveRoomModel) BumpVisitCount(ctx context.Context, roomNum string, delta int64) error {
        _, err := m.db.ExecContext(ctx,
                `UPDATE `+Tbl("live_room")+` SET visit_count = visit_count + ?, updated_at = strftime('%s','now') WHERE room_num=?`,
                delta, roomNum)
        return err
}

func scanLiveRoom(row *sql.Row) (*LiveRoom, error) {
        var r LiveRoom
        var anchorNick, anchorIcon, asstNick, asstIcon sql.NullString
        err := row.Scan(&r.UID, &r.RoomNum, &r.Title, &r.Contact, &r.Cover, &r.CustomCover,
                &r.Notice, &r.Detail, &r.LiveFLV, &r.LiveM3U8, &r.LiveStatus, &r.RoomStatus,
                &r.LiveType, &r.LiveTypeParent, &r.FocusCount, &r.FictitiousFocusCount,
                &r.VisitCount, &r.FictitiousVisitCount, &r.MarkType, &r.AssistantUID,
                &r.HD, &r.StreamType, &r.PushContent,
                &anchorNick, &anchorIcon,
                &asstNick, &asstIcon)
        if err != nil {
                if err == sql.ErrNoRows {
                        return nil, ErrNotFound
                }
                return nil, err
        }
        r.AnchorNickName = anchorNick.String
        r.AnchorIcon = anchorIcon.String
        r.AssistantNickName = asstNick.String
        r.AssistantIcon = asstIcon.String
        return &r, nil
}

func scanLiveRooms(rows *sql.Rows) ([]LiveRoom, error) {
        var out []LiveRoom
        for rows.Next() {
                var r LiveRoom
                var anchorNick, anchorIcon, asstNick, asstIcon sql.NullString
                if err := rows.Scan(&r.UID, &r.RoomNum, &r.Title, &r.Contact, &r.Cover, &r.CustomCover,
                        &r.Notice, &r.Detail, &r.LiveFLV, &r.LiveM3U8, &r.LiveStatus, &r.RoomStatus,
                        &r.LiveType, &r.LiveTypeParent, &r.FocusCount, &r.FictitiousFocusCount,
                        &r.VisitCount, &r.FictitiousVisitCount, &r.MarkType, &r.AssistantUID,
                        &r.HD, &r.StreamType, &r.PushContent,
                        &anchorNick, &anchorIcon,
                        &asstNick, &asstIcon); err != nil {
                        return nil, err
                }
                r.AnchorNickName = anchorNick.String
                r.AnchorIcon = anchorIcon.String
                r.AssistantNickName = asstNick.String
                r.AssistantIcon = asstIcon.String
                out = append(out, r)
        }
        return out, rows.Err()
}

func (r *LiveRoom) DebugString() string {
        return fmt.Sprintf("room=%s title=%s live=%d parent=%d", r.RoomNum, r.Title, r.LiveStatus, r.LiveTypeParent)
}
