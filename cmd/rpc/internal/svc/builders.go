package svc

// JSON shape builders for the YuYanTV protocol. These produce the exact JSON
// field names + grouping the zbyy client expects (per the Task 9-a/b/c
// worklog findings from /tmp/backend-zero).
//
// Shapes:
//   - matches.json         → { "0":[...], "1":[...], "2":[...], "5":[...], "hot":[...] }
//   - all_live_rooms.json  → { "0":[...], "<liveTypeId>":[...], "hot":[...] }
//   - live_types.json      → [{ liveTypeId, typeName, parentId, icon }]
//   - room/<n>/detail.json → { room:{...}, stream:{flv,hdFlv,m3u8,hdM3u8} }
//   - room/<n>/schedule.json → { matches:[MatchItem] }
//   - match/matches_<YYYYMMDD>.json → [MatchCatalogItem] (flat array, NOT grouped)
//   - match/recommend      → { count, pageNum, matches:[MatchCatalogItem] }
//
// categoryId in JSON = live_type_parent (NOT live_type child).
// matchTime = int64 ms UTC.
// matchStatusDesc = "" (intentional — backend-zero doesn't invent labels).

import (
        "context"
        "encoding/json"
        "sort"
        "strconv"
        "time"

        "apipro/common/model"

        "github.com/zeromicro/go-zero/core/logx"
)

// MatchCatalogItem — exact JSON shape from service.MatchCatalogItem.
type MatchCatalogItem struct {
        ScheduleID      int64             `json:"scheduleId"`
        MatchTime       int64             `json:"matchTime"`
        HostName        string            `json:"hostName"`
        GuestName       string            `json:"guestName"`
        HostIcon        string            `json:"hostIcon"`
        GuestIcon       string            `json:"guestIcon"`
        SubCateName     string            `json:"subCateName"`
        CategoryID      int64             `json:"categoryId"`
        CategoryName    string            `json:"categoryName,omitempty"`
        CategoryIcon    string            `json:"categoryIcon"`
        HostScore       int               `json:"hostScore"`
        GuestScore      int               `json:"guestScore"`
        MatchStatus     int               `json:"matchStatus,omitempty"`
        Status          int               `json:"status,omitempty"`
        MatchStatusDesc string            `json:"matchStatusDesc"` // always ""
        Anchors         []MatchAnchorItem `json:"anchors"`
}

type MatchAnchorItem struct {
        UID        int64              `json:"uid"`
        NickName   string             `json:"nickName"`
        Icon       string             `json:"icon"`
        CutOutIcon string             `json:"cutOutIcon,omitempty"`
        Anchor     MatchAnchorProfile `json:"anchor"`
}

type MatchAnchorProfile struct {
        RoomNum string `json:"roomNum"`
        Detail  string `json:"detail"`
        Notice  string `json:"notice"`
}

// MatchItem — short form (used by /room/<n>/schedule.json).
type MatchItem struct {
        ScheduleID   int64  `json:"scheduleId"`
        MatchTime    int64  `json:"matchTime"`
        HostName     string `json:"hostName"`
        GuestName    string `json:"guestName"`
        HostIcon     string `json:"hostIcon"`
        GuestIcon    string `json:"guestIcon"`
        SubCateName  string `json:"subCateName"`
        CategoryIcon string `json:"categoryIcon"`
}

// RoomResult — exact JSON shape from service.RoomResult.
type RoomResult struct {
        RoomNum              string       `json:"roomNum"`
        Title                string       `json:"title"`
        Cover                string       `json:"cover"`
        CutOutCustomCoverURL string       `json:"cutOutCustomCoverUrl"`
        MarkType             int          `json:"markType"`
        LiveStatus           int          `json:"liveStatus,omitempty"`
        HD                   int          `json:"hd,omitempty"`
        ViewCount            int64        `json:"viewCount"`
        FocusCount           int64        `json:"focusCount"`
        Anchor               AnchorResult `json:"anchor"`
}

type AnchorResult struct {
        UID        int64  `json:"uid"`
        NickName   string `json:"nickName"`
        Icon       string `json:"icon"`
        CutOutIcon string `json:"cutOutIcon,omitempty"`
        RoomNum    string `json:"roomNum"`
        Notice     string `json:"notice"`
}

// LiveTypeJSON — exact JSON shape from LiveTypeResult.
type LiveTypeJSON struct {
        LiveTypeID int64  `json:"liveTypeId"`
        TypeName   string `json:"typeName"`
        ParentID   int64  `json:"parentId"`
        Icon       string `json:"icon"`
}

// RoomDetailResult — used by /room/<n>/detail.json "room" field.
type RoomDetailResult struct {
        RoomNum       string               `json:"roomNum"`
        Title         string               `json:"title"`
        Contact       string               `json:"contact"`
        HD            int                  `json:"hd"`
        Cover         string               `json:"cover"`
        Notice        string               `json:"notice"`
        Detail        string               `json:"detail"`
        LiveFLV       string               `json:"liveFlv,omitempty"`
        LiveM3U8      string               `json:"liveM3u8,omitempty"`
        LiveStatus    int                  `json:"liveStatus,omitempty"`
        ViewCount     int64                `json:"viewCount"`
        FocusCount    int64                `json:"focusCount"`
        Anchor        AnchorResult         `json:"anchor"`
        AssistantUser *AssistantUserResult `json:"assistantUser,omitempty"`
}

type AssistantUserResult struct {
        UID      int64  `json:"uid"`
        NickName string `json:"nickName"`
        Icon     string `json:"icon"`
}

type PlayStreamURLs struct {
        FLV    string `json:"flv"`
        HdFLV  string `json:"hdFlv"`
        M3U8   string `json:"m3u8"`
        HdM3U8 string `json:"hdM3u8"`
}

// GiftRankItem — used by /room/<n>/gift_rank.json (bare array).
type GiftRankItem struct {
        User         UserInfoResult `json:"user"`
        Contribution int64          `json:"contribution"`
}

// UserInfoResult — exact JSON shape from service.UserInfoResult.
type UserInfoResult struct {
        UID         int64         `json:"uid"`
        NickName    string        `json:"nickName"`
        Icon        string        `json:"icon"`
        CutOutIcon  string        `json:"cutOutIcon"`
        UserType    int           `json:"userType"`
        Score       int64         `json:"score"`
        Grow        int64         `json:"grow"`
        GrowDTO     GrowDTOResult `json:"growDto"`
        Gender      int           `json:"gender"`
        Birthday    int64         `json:"birthday"`
        LoginName   string        `json:"loginName"`
        Phone       string        `json:"phone"`
        CountryCode string        `json:"countryCode"`
}

type GrowDTOResult struct {
        ID          int64  `json:"id"`
        Name        string `json:"name"`
        NextMinGrom int64  `json:"nextMinGrom"` // intentional legacy misspelling
}

// AuthResponse is the response for login/register/refresh/guest endpoints.
type AuthResponse struct {
        AccessToken  string         `json:"accessToken"`
        SessionID    string         `json:"sessionId"`
        RefreshToken string         `json:"refreshToken,omitempty"`
        UserInfo     UserInfoResult `json:"userInfo"`
        URLs         map[string]any `json:"urls,omitempty"`
        Phone        string         `json:"phone,omitempty"`
        CountryCode  int            `json:"countryCode,omitempty"`
        LoginName    string         `json:"loginName,omitempty"`
}

// ----- Builders -----

// BuildMatchCatalog returns the grouped map for matches.json:
//   { "0":[...], "1":[...], "2":[...], "5":[...], "hot":[...] }
//
// Per-type cap = 30 items. "0" is the merged "all" group (dedup by scheduleID,
// sorted by matchTime asc, capped at 30). "hot" is the recommend list (≤8).
func BuildMatchCatalog(ctx context.Context, m *Models) map[string][]MatchCatalogItem {
        out := map[string][]MatchCatalogItem{}
        for _, parent := range []int64{1, 2, 5} {
                rows, err := m.Matches.ListCatalog(ctx, []int64{parent}, 30)
                if err != nil {
                        logx.Errorf("BuildMatchCatalog: ListCatalog(parent=%d): %v", parent, err)
                        out[strconv.FormatInt(parent, 10)] = []MatchCatalogItem{}
                        continue
                }
                out[strconv.FormatInt(parent, 10)] = GroupCatalogRows(rows)
        }
        // "0" = merged all (dedup by scheduleID, sort by matchTime asc, cap 30)
        all := map[int64]MatchCatalogItem{}
        for _, items := range out {
                for _, it := range items {
                        if _, ok := all[it.ScheduleID]; !ok {
                                all[it.ScheduleID] = it
                        }
                }
        }
        allList := make([]MatchCatalogItem, 0, len(all))
        for _, it := range all {
                allList = append(allList, it)
        }
        sort.Slice(allList, func(i, j int) bool { return allList[i].MatchTime < allList[j].MatchTime })
        if len(allList) > 30 {
                allList = allList[:30]
        }
        out["0"] = allList
        out["hot"] = BuildRecommend(ctx, m)
        return out
}

// BuildRecommend returns ≤8 hot catalog items (with anchors).
func BuildRecommend(ctx context.Context, m *Models) []MatchCatalogItem {
        rows, err := m.Matches.ListRecommend(ctx, 8)
        if err != nil {
                return []MatchCatalogItem{}
        }
        return GroupCatalogRows(rows)
}

// BuildMatchByDate returns a flat array (NOT grouped) of catalog items for a
// specific date (YYYYMMDD).
func BuildMatchByDate(ctx context.Context, m *Models, date string) []MatchCatalogItem {
        rows, err := m.Matches.ListByDate(ctx, date, 100)
        if err != nil {
                return []MatchCatalogItem{}
        }
        return GroupCatalogRows(rows)
}

// GroupCatalogRows dedups by schedule_id, merges anchors, sorts by matchTime asc.
func GroupCatalogRows(rows []model.MatchCatalogRow) []MatchCatalogItem {
        byID := map[int64]*MatchCatalogItem{}
        order := []int64{}
        for _, r := range rows {
                it, ok := byID[r.ScheduleID]
                if !ok {
                        it = &MatchCatalogItem{
                                ScheduleID:      r.ScheduleID,
                                MatchTime:       model.MatchTimeToMS(r.MatchTime),
                                HostName:        r.HostName,
                                GuestName:       r.GuestName,
                                HostIcon:        r.HostIcon,
                                GuestIcon:       r.GuestIcon,
                                SubCateName:     r.SubCateName,
                                CategoryID:      r.LiveTypeParent,
                                CategoryName:    r.CategoryName,
                                CategoryIcon:    r.CategoryIcon,
                                HostScore:       r.HostScore,
                                GuestScore:      r.GuestScore,
                                MatchStatus:     r.MatchStatus,
                                Status:          1,
                                MatchStatusDesc: "", // intentional — backend-zero never invents labels
                                Anchors:         []MatchAnchorItem{},
                        }
                        byID[r.ScheduleID] = it
                        order = append(order, r.ScheduleID)
                }
                if r.AnchorUID > 0 {
                        it.Anchors = append(it.Anchors, MatchAnchorItem{
                                UID:      r.AnchorUID,
                                NickName: r.AnchorNickName,
                                Icon:     r.AnchorIcon,
                                Anchor: MatchAnchorProfile{
                                        RoomNum: r.RoomNum,
                                        Detail:  r.RoomDetail,
                                        Notice:  r.RoomNotice,
                                },
                        })
                }
        }
        out := make([]MatchCatalogItem, 0, len(order))
        for _, id := range order {
                out = append(out, *byID[id])
        }
        sort.Slice(out, func(i, j int) bool { return out[i].MatchTime < out[j].MatchTime })
        return out
}

// BuildAllLiveRooms returns the grouped map for all_live_rooms.json:
//   { "0":[...], "<liveTypeId>":[...], "hot":[...] }
func BuildAllLiveRooms(ctx context.Context, m *Models) map[string]any {
        out := map[string]any{}
        hot, err := m.Rooms.ListHot(ctx, 50)
        if err == nil {
                out["hot"] = RoomsToResults(hot)
        } else {
                out["hot"] = []RoomResult{}
        }
        lts, err := m.LiveTypes.ListTopLevel(ctx)
        if err == nil {
                for _, lt := range lts {
                        rooms, _ := m.Rooms.ListByType(ctx, lt.LiveTypeID, 50)
                        out[strconv.FormatInt(lt.LiveTypeID, 10)] = RoomsToResults(rooms)
                }
        }
        all, err := m.Rooms.ListAllVisible(ctx, 200)
        if err == nil {
                out["0"] = RoomsToResults(all)
        } else {
                out["0"] = []RoomResult{}
        }
        return out
}

// RoomsToResults converts []LiveRoom to []RoomResult.
func RoomsToResults(rooms []model.LiveRoom) []RoomResult {
        out := make([]RoomResult, 0, len(rooms))
        for _, r := range rooms {
                out = append(out, RoomResult{
                        RoomNum:              r.RoomNum,
                        Title:                r.Title,
                        Cover:                r.Cover,
                        CutOutCustomCoverURL: "", // intentional — TODO U-LV2
                        MarkType:             r.MarkType,
                        LiveStatus:           r.LiveStatus,
                        HD:                   r.HD,
                        ViewCount:            r.VisitCount + r.FictitiousVisitCount,
                        FocusCount:           r.FocusCount + r.FictitiousFocusCount,
                        Anchor: AnchorResult{
                                UID:      r.UID,
                                NickName: r.AnchorNickName,
                                Icon:     r.AnchorIcon,
                                RoomNum:  r.RoomNum,
                                Notice:   r.Notice,
                        },
                })
        }
        return out
}

// BuildLiveTypesJSON returns the LiveTypeJSON list for live_types.json.
func BuildLiveTypesJSON(lts []model.LiveType) []LiveTypeJSON {
        out := make([]LiveTypeJSON, 0, len(lts))
        for _, lt := range lts {
                out = append(out, LiveTypeJSON{
                        LiveTypeID: lt.LiveTypeID,
                        TypeName:   lt.TypeName,
                        ParentID:   lt.ParentID,
                        Icon:       lt.Icon,
                })
        }
        return out
}

// BuildRoomDetail returns the {room, stream} object for /room/<n>/detail.json.
func BuildRoomDetail(ctx context.Context, m *Models, roomNum string) map[string]any {
        r, err := m.Rooms.FindByRoomNum(ctx, roomNum)
        if err != nil {
                return nil
        }
        room := RoomDetailResult{
                RoomNum:    r.RoomNum,
                Title:      r.Title,
                Contact:    r.Contact,
                HD:         r.HD,
                Cover:      r.Cover,
                Notice:     r.Notice,
                Detail:     r.Detail,
                LiveFLV:    r.LiveFLV,
                LiveM3U8:   r.LiveM3U8,
                LiveStatus: r.LiveStatus,
                ViewCount:  r.VisitCount + r.FictitiousVisitCount,
                FocusCount: r.FocusCount + r.FictitiousFocusCount,
                Anchor: AnchorResult{
                        UID:      r.UID,
                        NickName: r.AnchorNickName,
                        Icon:     r.AnchorIcon,
                        RoomNum:  r.RoomNum,
                        Notice:   r.Notice,
                },
        }
        if r.AssistantUID > 0 {
                room.AssistantUser = &AssistantUserResult{
                        UID:      r.AssistantUID,
                        NickName: r.AssistantNickName,
                        Icon:     r.AssistantIcon,
                }
        }
        stream := PlayStreamURLs{
                FLV:    r.LiveFLV,
                HdFLV:  r.LiveFLV,
                M3U8:   r.LiveM3U8,
                HdM3U8: r.LiveM3U8,
        }
        return map[string]any{
                "room":   room,
                "stream": stream,
        }
}

// BuildRoomSchedule returns {matches:[MatchItem]} for /room/<n>/schedule.json.
func BuildRoomSchedule(ctx context.Context, m *Models, roomNum string) map[string][]MatchItem {
        rows, err := m.Matches.ListByRoom(ctx, roomNum, 50)
        if err != nil {
                return map[string][]MatchItem{"matches": {}}
        }
        out := make([]MatchItem, 0, len(rows))
        seen := map[int64]bool{}
        for _, r := range rows {
                if seen[r.ScheduleID] {
                        continue
                }
                seen[r.ScheduleID] = true
                out = append(out, MatchItem{
                        ScheduleID:   r.ScheduleID,
                        MatchTime:    model.MatchTimeToMS(r.MatchTime),
                        HostName:     r.HostName,
                        GuestName:    r.GuestName,
                        HostIcon:     r.HostIcon,
                        GuestIcon:    r.GuestIcon,
                        SubCateName:  r.SubCateName,
                        CategoryIcon: r.CategoryIcon,
                })
        }
        return map[string][]MatchItem{"matches": out}
}

// BuildGiftRank returns the bare array for /room/<n>/gift_rank.json.
func BuildGiftRank(ctx context.Context, m *Models, roomNum string) []GiftRankItem {
        ranks, err := m.GiftRanks.ListTopByRoom(ctx, roomNum, 10)
        if err != nil {
                return []GiftRankItem{}
        }
        out := make([]GiftRankItem, 0, len(ranks))
        for _, r := range ranks {
                out = append(out, GiftRankItem{
                        User: UserInfoResult{
                                UID:      r.UID,
                                NickName: r.NickName,
                                Icon:     r.Icon,
                        },
                        Contribution: r.Score,
                })
        }
        return out
}

// BuildUserInfo converts a User row to UserInfoResult.
func BuildUserInfo(ctx context.Context, m *Models, u *model.User) UserInfoResult {
        grow, _ := m.Users.FindUserGrowForValue(ctx, u.Grow)
        var birthdayMS int64
        if u.Birthday.Valid {
                birthdayMS = u.Birthday.Time.UnixMilli()
        }
        ccStr := u.CountryCode
        if ccStr != "" && ccStr[0] != '+' {
                ccStr = "+" + ccStr
        }
        growDTO := GrowDTOResult{}
        if grow != nil {
                growDTO = GrowDTOResult{
                        ID:          grow.ID,
                        Name:        grow.Name,
                        NextMinGrom: grow.NextMinGrow, // intentional misspelling
                }
        }
        return UserInfoResult{
                UID:         u.UID,
                NickName:    u.NickName,
                Icon:        u.Icon,
                CutOutIcon:  "", // not a DB column in the rebuild
                UserType:    int(u.UserType),
                Score:       u.Score,
                Grow:        u.Grow,
                GrowDTO:     growDTO,
                Gender:      int(u.Gender),
                Birthday:    birthdayMS,
                LoginName:   u.LoginName,
                Phone:       u.Phone,
                CountryCode: ccStr,
        }
}

// jsonBytes marshals v, returning nil on error (never panics).
func jsonBytes(v any) json.RawMessage {
        b, err := json.Marshal(v)
        if err != nil {
                return nil
        }
        return b
}

// todayKey returns YYYYMMDD for time.Now().
func todayKey() string { return time.Now().Format("20060102") }
