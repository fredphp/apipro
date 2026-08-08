package model

// LiveType model — reads from `live_type` table.
//
// Top-level live_type_id values: 1=football, 2=basketball, 5=analysis.
// Child rows exist too (parent_id != 0).

import (
	"context"
	"database/sql"
	"fmt"
)

type LiveType struct {
	LiveTypeID int64  `json:"liveTypeId"`
	ParentID   int64  `json:"parentId"`
	TypeName   string `json:"typeName"`
	Icon       string `json:"icon"`
	Status     int32  `json:"status"`
	Sort       int32  `json:"sort"`
}

type LiveTypeModel struct {
	db *sql.DB
}

func NewLiveTypeModel(db *sql.DB) *LiveTypeModel { return &LiveTypeModel{db: db} }

// ListTopLevel returns top-level (parent_id=0) live types, status=1, ordered
// by sort DESC, live_type_id ASC (matches backend-zero ordering).
func (m *LiveTypeModel) ListTopLevel(ctx context.Context) ([]LiveType, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT live_type_id, parent_id, type_name, icon, status, sort
		 FROM live_type WHERE status=1 AND parent_id=0
		 ORDER BY sort DESC, live_type_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLiveTypes(rows)
}

// FindByID returns a live type by ID.
func (m *LiveTypeModel) FindByID(ctx context.Context, id int64) (*LiveType, error) {
	row := m.db.QueryRowContext(ctx,
		`SELECT live_type_id, parent_id, type_name, icon, status, sort
		 FROM live_type WHERE live_type_id=?`, id)
	var lt LiveType
	if err := row.Scan(&lt.LiveTypeID, &lt.ParentID, &lt.TypeName, &lt.Icon, &lt.Status, &lt.Sort); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &lt, nil
}

// FindByParent returns child types for a parent.
func (m *LiveTypeModel) FindByParent(ctx context.Context, parentID int64) ([]LiveType, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT live_type_id, parent_id, type_name, icon, status, sort
		 FROM live_type WHERE parent_id=? AND status=1
		 ORDER BY sort DESC, live_type_id ASC`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLiveTypes(rows)
}

func scanLiveTypes(rows *sql.Rows) ([]LiveType, error) {
	var out []LiveType
	for rows.Next() {
		var lt LiveType
		if err := rows.Scan(&lt.LiveTypeID, &lt.ParentID, &lt.TypeName, &lt.Icon, &lt.Status, &lt.Sort); err != nil {
			return nil, err
		}
		out = append(out, lt)
	}
	return out, rows.Err()
}

func (lt *LiveType) DebugString() string {
	return fmt.Sprintf("id=%d parent=%d name=%s", lt.LiveTypeID, lt.ParentID, lt.TypeName)
}
