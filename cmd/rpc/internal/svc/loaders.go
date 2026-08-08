package svc

// DB loaders — read from MySQL/SQLite via models and convert to the fixture
// types that the cache layer expects. This keeps the cache + RPC contract
// identical whether the data source is fixtures or a real database.

import (
        "context"
        "time"

        "apipro/common/model"
        "apipro/pkg/fixture"
)

// ---- Match loaders ----

func LoadMatchesByDate(ctx context.Context, m *Models, date string) ([]fixture.MatchItem, error) {
        matches, err := m.Matches.ListByDate(ctx, date)
        if err != nil {
                return nil, err
        }
        return matchesToFixture(ctx, m, matches), nil
}

func LoadRecommend(ctx context.Context, m *Models) ([]fixture.MatchItem, error) {
        matches, err := m.Matches.ListRecommend(ctx, 5)
        if err != nil {
                return nil, err
        }
        return matchesToFixture(ctx, m, matches), nil
}

func LoadMatchByID(ctx context.Context, m *Models, id string) (fixture.MatchItem, error) {
        mm, err := m.Matches.FindByID(ctx, id)
        if err != nil {
                return fixture.MatchItem{}, err
        }
        list := matchesToFixture(ctx, m, []model.Match{*mm})
        if len(list) == 0 {
                return fixture.MatchItem{}, nil
        }
        return list[0], nil
}

func LoadMatchesByCate(ctx context.Context, m *Models, cate string) ([]fixture.MatchItem, error) {
        matches, err := m.Matches.ListByCate(ctx, cate)
        if err != nil {
                return nil, err
        }
        return matchesToFixture(ctx, m, matches), nil
}

func LoadMatchesByAnchorRoom(ctx context.Context, m *Models, roomNum string) ([]fixture.MatchItem, error) {
        matches, err := m.Matches.ListByAnchorRoom(ctx, roomNum)
        if err != nil {
                return nil, err
        }
        return matchesToFixture(ctx, m, matches), nil
}

func matchesToFixture(ctx context.Context, m *Models, matches []model.Match) []fixture.MatchItem {
        out := make([]fixture.MatchItem, 0, len(matches))
        for _, mm := range matches {
                anchors := loadAnchorsForMatch(ctx, m, mm.ScheduleId)
                out = append(out, fixture.MatchItem{
                        ScheduleId:        mm.ScheduleId,
                        SubCateName:       mm.SubCateName,
                        CateName:          mm.CateName,
                        MatchTime:         parseMatchTimeMs(mm.MatchTime),
                        HostName:          mm.HostName,
                        HostIcon:          mm.HostIcon,
                        GuestName:         mm.GuestName,
                        GuestIcon:         mm.GuestIcon,
                        Venue:             mm.Venue,
                        Status:            mm.Status,
                        ReservationStatus: mm.ReservationStatus,
                        Anchors:           anchors,
                        CategoryId:        cateNameToId(mm.CateName),
                        CategoryName:      mm.CateName,
                        MatchStatusDesc:   statusToDesc(mm.Status),
                })
        }
        return out
}

// parseMatchTimeMs parses a DB match_time string ("2006-01-02 15:04:05") into
// a millisecond timestamp. zbyy client treats matchTime as a number.
func parseMatchTimeMs(s string) int64 {
        if s == "" {
                return 0
        }
        for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", time.RFC3339} {
                if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
                        return t.UnixMilli()
                }
        }
        return 0
}

// cateNameToId maps a cate_name string to the zbyy categoryId.
// 1=football, 2=basketball, -1=other.
func cateNameToId(cate string) int32 {
        switch cate {
        case "足球", "football":
                return 1
        case "篮球", "basketball":
                return 2
        }
        return -1
}

// statusToDesc maps an internal status to a zbyy-style localized description.
func statusToDesc(status string) string {
        switch status {
        case "living":
                return "正在直播"
        case "over":
                return "已结束"
        case "not_started":
                return "今日直播"
        }
        return ""
}

func loadAnchorsForMatch(ctx context.Context, m *Models, matchId string) []fixture.Commentator {
        anchors, err := m.Anchors.ListByMatch(ctx, matchId)
        if err != nil {
                return []fixture.Commentator{}
        }
        return anchorsToFixture(anchors)
}

// ---- Anchor / Commentator loaders ----

func LoadCommentators(ctx context.Context, m *Models) ([]fixture.Commentator, error) {
        anchors, err := m.Anchors.ListAll(ctx)
        if err != nil {
                return nil, err
        }
        return anchorsToFixture(anchors), nil
}

func LoadHotAnchors(ctx context.Context, m *Models, n int) ([]fixture.Commentator, error) {
        anchors, err := m.Anchors.ListHot(ctx, n)
        if err != nil {
                return nil, err
        }
        return anchorsToFixture(anchors), nil
}

func LoadCommentator(ctx context.Context, m *Models, uid string) (fixture.Commentator, error) {
        a, err := m.Anchors.FindByUid(ctx, uid)
        if err != nil {
                return fixture.Commentator{}, err
        }
        return anchorToFixture(*a), nil
}

func anchorsToFixture(anchors []model.Anchor) []fixture.Commentator {
        out := make([]fixture.Commentator, 0, len(anchors))
        for _, a := range anchors {
                out = append(out, anchorToFixture(a))
        }
        return out
}

func anchorToFixture(a model.Anchor) fixture.Commentator {
        return fixture.Commentator{
                Uid:        a.Uid,
                NickName:   a.NickName,
                Icon:       a.Icon,
                CutOutIcon: a.CutOutIcon,
                Intro:      a.Intro,
                Fans:       a.Fans,
                Follow:     a.Follow,
                Hot:        a.Hot,
                Anchor: fixture.Anchor{
                        Uid:     a.Uid,
                        RoomNum: a.RoomNum,
                        Detail:  a.Detail,
                        Notice:  a.Notice,
                        Live:    a.Live,
                },
        }
}

// ---- Room loaders ----

func LoadRoomDetail(ctx context.Context, m *Models, roomNum string) (fixture.RoomDetail, error) {
        r, err := m.Rooms.FindByRoomNum(ctx, roomNum)
        if err != nil {
                return fixture.RoomDetail{}, err
        }
        anchor, _ := m.Anchors.FindByUid(ctx, r.AnchorUid)
        anchorF := fixture.Commentator{}
        if anchor != nil {
                anchorF = anchorToFixture(*anchor)
        }
        return fixture.RoomDetail{
                RoomNum:    r.RoomNum,
                Title:      r.Title,
                Cover:      r.Cover,
                Live:       r.Live,
                ViewNum:    r.ViewNum,
                LiveType:   r.LiveType,
                Anchor:     anchorF,
                StreamUrls: r.StreamUrls,
                Notice:     r.Notice,
                Tags:       r.Tags,
        }, nil
}

func LoadLiveRooms(ctx context.Context, m *Models) ([]fixture.LiveRoom, error) {
        rooms, err := m.Rooms.ListLive(ctx)
        if err != nil {
                return nil, err
        }
        out := make([]fixture.LiveRoom, 0, len(rooms))
        for _, r := range rooms {
                anchor, _ := m.Anchors.FindByUid(ctx, r.AnchorUid)
                anchorF := fixture.Commentator{}
                if anchor != nil {
                        anchorF = anchorToFixture(*anchor)
                }
                liveStatus := int32(0)
                if r.Live {
                        liveStatus = 1
                }
                out = append(out, fixture.LiveRoom{
                        RoomNum:              r.RoomNum,
                        Title:                r.Title,
                        Cover:                r.Cover,
                        Anchor:               anchorF,
                        ViewNum:              r.ViewNum,
                        LiveType:             r.LiveType,
                        CateName:             r.CateName,
                        ViewCount:            r.ViewNum,
                        CutOutCustomCoverUrl: r.Cover,
                        LiveStatus:           liveStatus,
                })
        }
        return out, nil
}

func LoadLiveTypes(ctx context.Context, m *Models) ([]fixture.LiveType, error) {
        lts, err := m.Matches.ListLiveTypes(ctx)
        if err != nil {
                return nil, err
        }
        out := make([]fixture.LiveType, 0, len(lts))
        for _, lt := range lts {
                out = append(out, fixture.LiveType{Code: lt.Code, Name: lt.Name, Icon: lt.Icon})
        }
        return out, nil
}

// ---- Rank loader ----

func LoadRoomRank(ctx context.Context, m *Models, roomNum string) ([]fixture.RoomRankItem, error) {
        ranks, err := m.Rooms.ListRank(ctx, roomNum)
        if err != nil {
                return nil, err
        }
        out := make([]fixture.RoomRankItem, 0, len(ranks))
        for _, r := range ranks {
                out = append(out, fixture.RoomRankItem{
                        Uid: r.Uid, NickName: r.NickName, Icon: r.Icon,
                        Score: r.Score, Rank: r.RankNo,
                })
        }
        return out, nil
}

// seedMatchStatus computes a match status from its time (for the seed tool).
func seedMatchStatus(matchTime string) string {
        t, err := time.Parse("2006-01-02 15:04:05", matchTime)
        if err != nil {
                return "not_started"
        }
        now := time.Now()
        if now.Before(t) {
                return "not_started"
        }
        if now.Sub(t) < 2*time.Hour+30*time.Minute {
                return "living"
        }
        return "over"
}
