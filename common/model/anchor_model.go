package model

// Anchor (commentator) model — reads from the `anchors` table.

import (
	"context"
	"database/sql"
	"fmt"
)

type Anchor struct {
	Uid        string
	NickName   string
	Icon       string
	CutOutIcon string
	Intro      string
	Fans       int64
	Follow     int64
	Hot        int64
	RoomNum    string
	Detail     string
	Notice     string
	Live       bool
	CreatedAt  int64
}

type AnchorModel struct {
	db *sql.DB
}

func NewAnchorModel(db *sql.DB) *AnchorModel { return &AnchorModel{db: db} }

const anchorCols = `uid, nick_name, icon, cut_out_icon, intro, fans, follow, hot, room_num, detail, notice, live, created_at`

func (m *AnchorModel) ListAll(ctx context.Context) ([]Anchor, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT `+anchorCols+` FROM anchors ORDER BY hot DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAnchors(rows)
}

func (m *AnchorModel) ListHot(ctx context.Context, limit int) ([]Anchor, error) {
	if limit <= 0 {
		limit = 6
	}
	rows, err := m.db.QueryContext(ctx, `SELECT `+anchorCols+` FROM anchors ORDER BY hot DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAnchors(rows)
}

func (m *AnchorModel) FindByUid(ctx context.Context, uid string) (*Anchor, error) {
	row := m.db.QueryRowContext(ctx, `SELECT `+anchorCols+` FROM anchors WHERE uid=?`, uid)
	var a Anchor
	var live int32
	err := row.Scan(&a.Uid, &a.NickName, &a.Icon, &a.CutOutIcon, &a.Intro,
		&a.Fans, &a.Follow, &a.Hot, &a.RoomNum, &a.Detail, &a.Notice, &live, &a.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	a.Live = live != 0
	return &a, nil
}

func (m *AnchorModel) FindByRoomNum(ctx context.Context, roomNum string) (*Anchor, error) {
	row := m.db.QueryRowContext(ctx, `SELECT `+anchorCols+` FROM anchors WHERE room_num=?`, roomNum)
	var a Anchor
	var live int32
	err := row.Scan(&a.Uid, &a.NickName, &a.Icon, &a.CutOutIcon, &a.Intro,
		&a.Fans, &a.Follow, &a.Hot, &a.RoomNum, &a.Detail, &a.Notice, &live, &a.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	a.Live = live != 0
	return &a, nil
}

func (m *AnchorModel) ListByMatch(ctx context.Context, matchId string) ([]Anchor, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT a.`+anchorCols+` FROM anchors a
		 INNER JOIN match_anchors ma ON ma.anchor_uid = a.uid
		 WHERE ma.match_id = ?`, matchId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAnchors(rows)
}

func scanAnchors(rows *sql.Rows) ([]Anchor, error) {
	var out []Anchor
	for rows.Next() {
		var a Anchor
		var live int32
		if err := rows.Scan(&a.Uid, &a.NickName, &a.Icon, &a.CutOutIcon, &a.Intro,
			&a.Fans, &a.Follow, &a.Hot, &a.RoomNum, &a.Detail, &a.Notice, &live, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.Live = live != 0
		out = append(out, a)
	}
	return out, rows.Err()
}

func (a *Anchor) DebugString() string {
	return fmt.Sprintf("uid=%s nick=%s room=%s live=%v", a.Uid, a.NickName, a.RoomNum, a.Live)
}
