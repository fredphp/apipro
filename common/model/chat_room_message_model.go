package model

// ChatRoomMessage model — reads/writes `chat_room_message` table.
//
// Used by WS chat for history replay + persistent log. The msg ID comes from
// Redis INCR yuyan:chat:message_id (bootstrapped from MAX(chat_room_message_id)).

import (
	"context"
	"database/sql"
	"time"
)

type ChatRoomMessage struct {
	ID        int64
	SendUID   int64
	RoomNum   string
	SendTime  time.Time
	Content   string
	Type      int32 // 1=text 2=gift 3=system
	IP        string
	Status    int32 // 1=visible 0=deleted
	CreatedAt int64
}

type ChatRoomMessageModel struct {
	db *sql.DB
}

func NewChatRoomMessageModel(db *sql.DB) *ChatRoomMessageModel { return &ChatRoomMessageModel{db: db} }

// Insert writes a chat message row.
func (m *ChatRoomMessageModel) Insert(ctx context.Context, msg *ChatRoomMessage) error {
	if msg.SendTime.IsZero() {
		msg.SendTime = time.Now()
	}
	if msg.CreatedAt == 0 {
		msg.CreatedAt = time.Now().Unix()
	}
	if msg.Status == 0 {
		msg.Status = 1
	}
	_, err := m.db.ExecContext(ctx, `
		INSERT INTO chat_room_message (chat_room_message_id, send_uid, room_num, send_time, content, type, ip, status, created_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		msg.ID, msg.SendUID, msg.RoomNum, msg.SendTime, msg.Content, msg.Type, msg.IP, msg.Status, msg.CreatedAt)
	return err
}

// ListRecentByRoom returns the most recent N visible messages of types 1/2
// for a room, oldest-first (caller may reverse for display).
func (m *ChatRoomMessageModel) ListRecentByRoom(ctx context.Context, roomNum string, limit int) ([]ChatRoomMessage, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := m.db.QueryContext(ctx, `
		SELECT chat_room_message_id, send_uid, room_num, send_time, content, type, ip, status, created_at
		FROM chat_room_message
		WHERE room_num=? AND COALESCE(status,1)=1 AND type IN (1,2)
		ORDER BY send_time DESC LIMIT ?`, roomNum, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChatRoomMessage
	for rows.Next() {
		var m ChatRoomMessage
		if err := rows.Scan(&m.ID, &m.SendUID, &m.RoomNum, &m.SendTime, &m.Content, &m.Type, &m.IP, &m.Status, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MaxID returns the highest chat_room_message_id (used to bootstrap the Redis
// INCR counter). Returns 0 on empty table.
func (m *ChatRoomMessageModel) MaxID(ctx context.Context) (int64, error) {
	var maxID sql.NullInt64
	err := m.db.QueryRowContext(ctx, `SELECT MAX(chat_room_message_id) FROM chat_room_message`).Scan(&maxID)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return maxID.Int64, nil
}
