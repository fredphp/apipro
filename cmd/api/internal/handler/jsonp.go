package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"apipro/cmd/api/internal/svc"
	"apipro/cmd/rpc/apiproClient"
)

// =============================================================
// JSONP plaintext endpoints
//
// All are GET, plaintext JSON (no encryption), with optional ?callback=xxx
// for JSONP wrapping. Response shape: <callback>({"data":<payload>})
// (or bare JSON if no callback query).
// =============================================================

// MatchesJSONPHandler — GET /matches.json
// Returns { "0":[...], "1":[...], "2":[...], "5":[...], "hot":[...] }
func MatchesJSONPHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp, err := callRPC(svcCtx, w, r, "matches_jsonp", "{}")
		if err != nil {
			return
		}
		serveJSONP(w, r, resp, "matches", svcCtx.Config.JsonpSnapshotDir, "matches.json")
	}
}

// AllLiveRoomsJSONPHandler — GET /all_live_rooms.json
func AllLiveRoomsJSONPHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp, err := callRPC(svcCtx, w, r, "live_all_rooms", "{}")
		if err != nil {
			return
		}
		serveJSONP(w, r, resp, "all_live_rooms", svcCtx.Config.JsonpSnapshotDir, "all_live_rooms.json")
	}
}

// LiveTypesJSONPHandler — GET /live_types.json
func LiveTypesJSONPHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp, err := callRPC(svcCtx, w, r, "live_types", "{}")
		if err != nil {
			return
		}
		serveJSONP(w, r, resp, "live_types", svcCtx.Config.JsonpSnapshotDir, "live_types.json")
	}
}

// HotAnchorJSONPHandler — GET /hot_anchor.json
func HotAnchorJSONPHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp, err := callRPC(svcCtx, w, r, "live_hot", "{}")
		if err != nil {
			return
		}
		// zbyy hot_anchor.json expects { anchors:[{nickName,icon,anchor:{roomNum}}] }
		// Our live_hot returns { hot:[RoomResult] }. Convert.
		var src struct {
			Hot []struct {
				NickName string `json:"title"`
				Anchor   struct {
					NickName string `json:"nickName"`
					Icon     string `json:"icon"`
					RoomNum  string `json:"roomNum"`
				} `json:"anchor"`
			} `json:"hot"`
		}
		_ = json.Unmarshal(resp.Result, &src)
		anchors := []map[string]any{}
		for _, h := range src.Hot {
			anchors = append(anchors, map[string]any{
				"nickName": h.Anchor.NickName,
				"icon":     h.Anchor.Icon,
				"anchor":   map[string]any{"roomNum": h.Anchor.RoomNum},
			})
		}
		body, _ := json.Marshal(map[string]any{"anchors": anchors})
		resp.Result = body
		serveJSONP(w, r, resp, "hot_anchor", svcCtx.Config.JsonpSnapshotDir, "hot_anchor.json")
	}
}

// RoomDetailJSONPHandler — GET /room/{roomNum}/detail.json
func RoomDetailJSONPHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roomNum := pathParam(r, "roomNum")
		if roomNum == "" {
			http.NotFound(w, r)
			return
		}
		param, _ := json.Marshal(map[string]string{"roomNum": roomNum})
		resp, err := callRPC(svcCtx, w, r, "room_detail", string(param))
		if err != nil {
			return
		}
		serveJSONP(w, r, resp, "detail", svcCtx.Config.JsonpSnapshotDir, "")
	}
}

// RoomScheduleJSONPHandler — GET /room/{roomNum}/schedule.json
func RoomScheduleJSONPHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roomNum := pathParam(r, "roomNum")
		if roomNum == "" {
			http.NotFound(w, r)
			return
		}
		param, _ := json.Marshal(map[string]string{"roomNum": roomNum})
		resp, err := callRPC(svcCtx, w, r, "room_schedule", string(param))
		if err != nil {
			return
		}
		cb := "schedule_" + roomNum
		serveJSONP(w, r, resp, cb, svcCtx.Config.JsonpSnapshotDir, "")
	}
}

// RoomGiftRankJSONPHandler — GET /room/{roomNum}/gift_rank.json
func RoomGiftRankJSONPHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roomNum := pathParam(r, "roomNum")
		if roomNum == "" {
			http.NotFound(w, r)
			return
		}
		param, _ := json.Marshal(map[string]string{"roomNum": roomNum})
		resp, err := callRPC(svcCtx, w, r, "room_gift_rank", string(param))
		if err != nil {
			return
		}
		serveJSONP(w, r, resp, "gift_rank", svcCtx.Config.JsonpSnapshotDir, "")
	}
}

// MatchDateJSONPHandler — GET /match/matches_YYYYMMDD.json (flat array)
// Path: /match/matches_<YYYYMMDD>.json
func MatchDateJSONPHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := pathParam(r, "name")
		// Extract YYYYMMDD from "matches_<YYYYMMDD>.json"
		if !strings.HasPrefix(name, "matches_") || !strings.HasSuffix(name, ".json") {
			http.NotFound(w, r)
			return
		}
		date := strings.TrimSuffix(strings.TrimPrefix(name, "matches_"), ".json")
		if len(date) != 8 {
			http.NotFound(w, r)
			return
		}
		param, _ := json.Marshal(map[string]string{"date": date})
		resp, err := callRPC(svcCtx, w, r, "match_byDate", string(param))
		if err != nil {
			return
		}
		// Date endpoint returns a flat array (NOT grouped); callback name = matches_<YYYYMMDD>
		cb := "matches_" + date
		serveJSONPFlat(w, r, resp, cb, svcCtx.Config.JsonpSnapshotDir, "match/matches_"+date+".json")
	}
}

// MatchRecommendJSONPHandler — GET /match_recommend.json
func MatchRecommendJSONPHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp, err := callRPC(svcCtx, w, r, "match_recommend", "{}")
		if err != nil {
			return
		}
		serveJSONP(w, r, resp, "match_recommend", svcCtx.Config.JsonpSnapshotDir, "match_recommend.json")
	}
}

// =============================================================
// Helpers
// =============================================================

// pathParam extracts a named path parameter from r.PathValue (Go 1.22+) with
// fallback to manual parsing.
func pathParam(r *http.Request, name string) string {
	if v := r.PathValue(name); v != "" {
		return v
	}
	// Manual fallback: parse the URL path.
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch name {
	case "roomNum":
		// /room/{roomNum}/detail.json → parts = ["room", "<roomNum>", "detail.json"]
		if len(parts) >= 2 {
			return parts[1]
		}
	case "name":
		// /match/{name} → parts = ["match", "<name>"]
		if len(parts) >= 2 {
			return parts[1]
		}
	}
	return ""
}

// serveJSONP writes a JSONP response. If a snapshot dir + filename are
// provided, the response is also written to disk for the snapshot job.
// The response shape is: <callback>({"data":<payload>})
// If no `callback` query is present, returns bare JSON: {"data":<payload>}
func serveJSONP(w http.ResponseWriter, r *http.Request, resp *apiproClient.CallResp, defaultCallback, snapDir, snapFile string) {
	if resp.Code != 200 && resp.Code != 0 {
		// On error, still emit the envelope with code+meg
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": resp.Code,
			"meg":  resp.Meg,
			"data": nil,
		})
		return
	}
	// Build payload: { "data": <result> }
	var data any = nil
	if len(resp.Result) > 0 {
		_ = json.Unmarshal(resp.Result, &data)
	}
	body, _ := json.Marshal(map[string]any{"data": data})

	// Write to snapshot dir.
	if snapDir != "" && snapFile != "" {
		writeSnapshot(snapDir, snapFile, body, defaultCallback)
	}

	// Serve JSONP or bare JSON.
	cb := r.URL.Query().Get("callback")
	if cb == "" {
		cb = defaultCallback
	}
	if r.URL.Query().Has("callback") && r.URL.Query().Get("callback") == "" {
		// ?callback= with empty value → bare JSON
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(body)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	_, _ = w.Write([]byte(cb + "("))
	_, _ = w.Write(body)
	_, _ = w.Write([]byte(")"))
}

// serveJSONPFlat writes a JSONP response where the payload is a BARE ARRAY
// (not wrapped in {"data":...}). This matches /match/matches_YYYYMMDD.json.
func serveJSONPFlat(w http.ResponseWriter, r *http.Request, resp *apiproClient.CallResp, defaultCallback, snapDir, snapFile string) {
	body := resp.Result
	if len(body) == 0 {
		body = []byte("[]")
	}
	if snapDir != "" && snapFile != "" {
		writeSnapshot(snapDir, snapFile, body, defaultCallback)
	}
	cb := r.URL.Query().Get("callback")
	if cb == "" {
		cb = defaultCallback
	}
	if r.URL.Query().Has("callback") && r.URL.Query().Get("callback") == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(body)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	_, _ = w.Write([]byte(cb + "("))
	_, _ = w.Write(body)
	_, _ = w.Write([]byte(")"))
}

// writeSnapshot writes a JSONP snapshot file atomically.
func writeSnapshot(dir, file string, body []byte, callback string) {
	full := filepath.Join(dir, file)
	_ = os.MkdirAll(filepath.Dir(full), 0o755)
	// Atomic-ish: write to temp then rename.
	tmp := full + ".tmp"
	content := []byte(callback + "(")
	content = append(content, body...)
	content = append(content, ')')
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, full)
}

// healthHandler — GET /health
func HealthHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(fmt.Sprintf(`{"status":"ok","ts":%d}`, time.Now().Unix())))
	}
}
