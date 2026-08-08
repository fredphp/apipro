package model

// RoomGiftRank model — reads from `room_gift_rank` table.

import (
        "context"
        "database/sql"
)

type RoomGiftRank struct {
        RoomNum  string
        UID      int64
        NickName string
        Icon     string
        Score    int64
        RankNo   int32
}

type RoomGiftRankModel struct {
        db *sql.DB
}

func NewRoomGiftRankModel(db *sql.DB) *RoomGiftRankModel { return &RoomGiftRankModel{db: db} }

func (m *RoomGiftRankModel) ListTopByRoom(ctx context.Context, roomNum string, limit int) ([]RoomGiftRank, error) {
        if limit <= 0 {
                limit = 10
        }
        // AUDIT-022: JOIN user table so nick_name/icon stay fresh when users
        // update their profile (the denormalized r.nick_name/r.icon is the
        // fallback only when the user row is missing).
        rows, err := m.db.QueryContext(ctx,
                `SELECT r.room_num, r.uid,
                        COALESCE(u.nick_name, r.nick_name),
                        COALESCE(u.icon, r.icon),
                        r.score, r.rank_no
                 FROM room_gift_rank r
                 LEFT JOIN user u ON u.uid = r.uid
                 WHERE r.room_num=? ORDER BY r.rank_no ASC LIMIT ?`, roomNum, limit)
        if err != nil {
                return nil, err
        }
        defer rows.Close()
        var out []RoomGiftRank
        for rows.Next() {
                var r RoomGiftRank
                if err := rows.Scan(&r.RoomNum, &r.UID, &r.NickName, &r.Icon, &r.Score, &r.RankNo); err != nil {
                        return nil, err
                }
                out = append(out, r)
        }
        return out, rows.Err()
}
