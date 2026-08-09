package model

// Room model — reads from the `rooms` table.
// stream_urls and tags are stored as JSON text and decoded here.

import (
        "context"
        "database/sql"
        "encoding/json"

        "github.com/zeromicro/go-zero/core/logx"
)

type Room struct {
        RoomNum    string
        Title      string
        Cover      string
        Live       bool
        ViewNum    int64
        LiveType   string
        AnchorUid  string
        StreamUrls []string
        Notice     string
        Tags       []string
        CateName   string
        CreatedAt  int64
}

type RoomModel struct {
        db *sql.DB
}

func NewRoomModel(db *sql.DB) *RoomModel { return &RoomModel{db: db} }

func (m *RoomModel) FindByRoomNum(ctx context.Context, roomNum string) (*Room, error) {
        row := m.db.QueryRowContext(ctx,
                `SELECT room_num, title, cover, live, view_num, live_type, anchor_uid, stream_urls, notice, tags, cate_name, created_at
                 FROM `+Tbl("rooms")+` WHERE room_num=?`, roomNum)
        var r Room
        var live int32
        var streamUrlsJSON, tagsJSON string
        err := row.Scan(&r.RoomNum, &r.Title, &r.Cover, &live, &r.ViewNum,
                &r.LiveType, &r.AnchorUid, &streamUrlsJSON, &r.Notice, &tagsJSON, &r.CateName, &r.CreatedAt)
        if err != nil {
                if err == sql.ErrNoRows {
                        return nil, ErrNotFound
                }
                return nil, err
        }
        r.Live = live != 0
        r.StreamUrls = decodeStrArray(streamUrlsJSON)
        r.Tags = decodeStrArray(tagsJSON)
        return &r, nil
}

func (m *RoomModel) ListLive(ctx context.Context) ([]Room, error) {
        rows, err := m.db.QueryContext(ctx,
                `SELECT room_num, title, cover, live, view_num, live_type, anchor_uid, stream_urls, notice, tags, cate_name, created_at
                 FROM `+Tbl("rooms")+` WHERE live=1 ORDER BY view_num DESC`)
        if err != nil {
                return nil, err
        }
        defer rows.Close()
        return scanRooms(rows)
}

func (m *RoomModel) ListAll(ctx context.Context) ([]Room, error) {
        rows, err := m.db.QueryContext(ctx,
                `SELECT room_num, title, cover, live, view_num, live_type, anchor_uid, stream_urls, notice, tags, cate_name, created_at
                 FROM `+Tbl("rooms")+` ORDER BY view_num DESC`)
        if err != nil {
                return nil, err
        }
        defer rows.Close()
        return scanRooms(rows)
}

func scanRooms(rows *sql.Rows) ([]Room, error) {
        var out []Room
        for rows.Next() {
                var r Room
                var live int32
                var streamUrlsJSON, tagsJSON string
                if err := rows.Scan(&r.RoomNum, &r.Title, &r.Cover, &live, &r.ViewNum,
                        &r.LiveType, &r.AnchorUid, &streamUrlsJSON, &r.Notice, &tagsJSON, &r.CateName, &r.CreatedAt); err != nil {
                        return nil, err
                }
                r.Live = live != 0
                r.StreamUrls = decodeStrArray(streamUrlsJSON)
                r.Tags = decodeStrArray(tagsJSON)
                out = append(out, r)
        }
        return out, rows.Err()
}

func decodeStrArray(jsonStr string) []string {
        if jsonStr == "" {
                return []string{}
        }
        var arr []string
        if err := json.Unmarshal([]byte(jsonStr), &arr); err != nil {
                logx.Errorf("decode json array %q: %v", jsonStr, err)
                return []string{}
        }
        if arr == nil {
                return []string{}
        }
        return arr
}

// Rank model
type RoomRank struct {
        Uid      string
        NickName string
        Icon     string
        Score    int64
        RankNo   int32
}

func (m *RoomModel) ListRank(ctx context.Context, roomNum string) ([]RoomRank, error) {
        rows, err := m.db.QueryContext(ctx,
                `SELECT uid, nick_name, icon, score, rank_no FROM `+Tbl("room_ranks")+` WHERE room_num=? ORDER BY rank_no ASC`, roomNum)
        if err != nil {
                return nil, err
        }
        defer rows.Close()
        var out []RoomRank
        for rows.Next() {
                var r RoomRank
                if err := rows.Scan(&r.Uid, &r.NickName, &r.Icon, &r.Score, &r.RankNo); err != nil {
                        return nil, err
                }
                out = append(out, r)
        }
        return out, rows.Err()
}
