package fixture

// In-memory data fixtures modeled exactly on the zbyy live-streaming platform data shapes.
// These seed the Redis cache on first access. In a production deployment you would replace
// the Load* functions with calls to the real upstream backend; the cache + RPC contract stay identical.

import (
        "fmt"
        "strconv"
        "sync"
        "time"
)

type Anchor struct {
        Uid     string `json:"uid"`
        RoomNum string `json:"roomNum"`
        Detail  string `json:"detail"`
        Notice  string `json:"notice"`
        Live    bool   `json:"live"`
}

type Commentator struct {
        Uid        string `json:"uid"`
        NickName   string `json:"nickName"`
        Icon       string `json:"icon"`
        CutOutIcon string `json:"cutOutIcon"`
        Intro      string `json:"intro"`
        Fans       int64  `json:"fans"`
        Follow     int64  `json:"follow"`
        Hot        int64  `json:"hot"`
        Anchor     Anchor `json:"anchor"`
}

type MatchItem struct {
        ScheduleId        string         `json:"scheduleId"`
        SubCateName       string         `json:"subCateName"`
        CateName          string         `json:"cateName"`
        MatchTime         int64          `json:"matchTime"` // ms timestamp
        HostName          string         `json:"hostName"`
        HostIcon          string         `json:"hostIcon"`
        GuestName         string         `json:"guestName"`
        GuestIcon         string         `json:"guestIcon"`
        Venue             string         `json:"venue"`
        Status            string         `json:"status"`
        ReservationStatus int32          `json:"reservationStatus"`
        Anchors           []Commentator  `json:"anchors"`
        CategoryId        int32          `json:"categoryId"`
        CategoryName      string         `json:"categoryName"`
        CategoryIcon      string         `json:"categoryIcon"`
        MatchStatusDesc   string         `json:"matchStatusDesc"`
        HostScore         int32          `json:"hostScore"`
        GuestScore        int32          `json:"guestScore"`
}

type RoomDetail struct {
        RoomNum    string        `json:"roomNum"`
        Title      string        `json:"title"`
        Cover      string        `json:"cover"`
        Live       bool          `json:"live"`
        ViewNum    int64         `json:"viewNum"`
        LiveType   string        `json:"liveType"`
        Anchor     Commentator   `json:"anchor"`
        StreamUrls []string      `json:"streamUrls"`
        Notice     string        `json:"notice"`
        Tags       []string      `json:"tags"`
}

type LiveRoom struct {
        RoomNum              string       `json:"roomNum"`
        Title                string       `json:"title"`
        Cover                string       `json:"cover"`
        Anchor               Commentator  `json:"anchor"`
        ViewNum              int64        `json:"viewNum"`
        LiveType             string       `json:"liveType"`
        CateName             string       `json:"cateName"`
        ViewCount            int64        `json:"viewCount"`
        CutOutCustomCoverUrl string       `json:"cutOutCustomCoverUrl"`
        MarkType             int32        `json:"markType"`
        LiveStatus           int32        `json:"liveStatus"`
}

type LiveType struct {
        Code string `json:"code"`
        Name string `json:"name"`
        Icon string `json:"icon"`
}

type UserRecord struct {
        Uid         string `json:"uid"`
        LoginName   string `json:"loginName"`
        NickName    string `json:"nickName"`
        Phone       string `json:"phone"`
        CountryCode string `json:"countryCode"`
        Password    string `json:"-"` // bcrypt hash, never serialized
        Grow        int64  `json:"grow"`
        Score       int64  `json:"score"`
        Level       int32  `json:"level"`
        Avatar      string `json:"avatar"`
        IsUser      int32  `json:"isUser"`
        CreatedAt   int64  `json:"createdAt"`
}

var (
        mu          sync.RWMutex
        anchors     []Commentator
        rooms       []RoomDetail
        lives       []LiveRoom
        liveTypes   []LiveType
        matchesByDay map[string][]MatchItem // date YYYYMMDD -> matches
        allMatches  []MatchItem
        rankByRoom  map[string][]RoomRankItem
        seedOnce    sync.Once
)

type RoomRankItem struct {
        Uid      string `json:"uid"`
        NickName string `json:"nickName"`
        Icon     string `json:"icon"`
        Score    int64  `json:"score"`
        Rank     int32  `json:"rank"`
}

func seed() {
        seedOnce.Do(func() {
                anchors = []Commentator{
                        {Uid: "A1001", NickName: "飞鱼解说", Icon: "https://cdn.zbyy.example/avatar/a1001.png", CutOutIcon: "https://cdn.zbyy.example/avatar/a1001_cut.png", Intro: "前职业球员，专注英超解说10年", Fans: 128000, Follow: 98000, Hot: 9527, Anchor: Anchor{Uid: "A1001", RoomNum: "1001", Detail: "每晚8点英超直播", Notice: "禁止刷屏、禁止广告", Live: true}},
                        {Uid: "A1002", NickName: "篮球小子", Icon: "https://cdn.zbyy.example/avatar/a1002.png", CutOutIcon: "https://cdn.zbyy.example/avatar/a1002_cut.png", Intro: "NBA/CBA深度分析", Fans: 86000, Follow: 64000, Hot: 7610, Anchor: Anchor{Uid: "A1002", RoomNum: "1002", Detail: "NBA专场", Notice: "文明观赛", Live: true}},
                        {Uid: "A1003", NickName: "绿茵观察", Icon: "https://cdn.zbyy.example/avatar/a1003.png", CutOutIcon: "https://cdn.zbyy.example/avatar/a1003_cut.png", Intro: "西甲、欧冠战术分析", Fans: 54000, Follow: 41000, Hot: 5230, Anchor: Anchor{Uid: "A1003", RoomNum: "1003", Detail: "西甲之夜", Notice: "理性讨论", Live: false}},
                        {Uid: "A1004", NickName: "斯诺克达人", Icon: "https://cdn.zbyy.example/avatar/a1004.png", CutOutIcon: "https://cdn.zbyy.example/avatar/a1004_cut.png", Intro: "斯诺克职业赛事解说", Fans: 21000, Follow: 18000, Hot: 2100, Anchor: Anchor{Uid: "A1004", RoomNum: "1004", Detail: "斯诺克直播", Notice: "安静观赛", Live: false}},
                        {Uid: "A1005", NickName: "中超前线", Icon: "https://cdn.zbyy.example/avatar/a1005.png", CutOutIcon: "https://cdn.zbyy.example/avatar/a1005_cut.png", Intro: "中超、亚冠现场报道", Fans: 39000, Follow: 30000, Hot: 3340, Anchor: Anchor{Uid: "A1005", RoomNum: "1005", Detail: "中超集锦", Notice: "禁止地域攻击", Live: true}},
                        {Uid: "A1006", NickName: "德甲工匠", Icon: "https://cdn.zbyy.example/avatar/a1006.png", CutOutIcon: "https://cdn.zbyy.example/avatar/a1006_cut.png", Intro: "德甲战术拆解", Fans: 47000, Follow: 35000, Hot: 4120, Anchor: Anchor{Uid: "A1006", RoomNum: "1006", Detail: "德甲周末", Notice: "文明互动", Live: false}},
                }
                findAnchor := func(uid string) Commentator {
                        for _, a := range anchors {
                                if a.Uid == uid {
                                        return a
                                }
                        }
                        return anchors[0]
                }

                liveTypes = []LiveType{
                        {Code: "football", Name: "足球", Icon: "https://cdn.zbyy.example/ico/football.png"},
                        {Code: "basketball", Name: "篮球", Icon: "https://cdn.zbyy.example/ico/basketball.png"},
                        {Code: "snooker", Name: "斯诺克", Icon: "https://cdn.zbyy.example/ico/snooker.png"},
                        {Code: "other", Name: "其它", Icon: "https://cdn.zbyy.example/ico/other.png"},
                }

                rooms = []RoomDetail{
                        {RoomNum: "1001", Title: "英超焦点战: 曼联 vs 利物浦", Cover: "https://cdn.zbyy.example/cover/1001.jpg", Live: true, ViewNum: 38211, LiveType: "football", Anchor: findAnchor("A1001"), StreamUrls: []string{"https://live.zbyy.example/1001/hd.m3u8", "https://live.zbyy.example/1001/sd.m3u8"}, Notice: "文明观赛，禁止刷屏", Tags: []string{"英超", "曼联", "利物浦"}},
                        {RoomNum: "1002", Title: "NBA常规赛: 湖人 vs 勇士", Cover: "https://cdn.zbyy.example/cover/1002.jpg", Live: true, ViewNum: 51209, LiveType: "basketball", Anchor: findAnchor("A1002"), StreamUrls: []string{"https://live.zbyy.example/1002/hd.m3u8"}, Notice: "理性讨论", Tags: []string{"NBA", "湖人", "勇士"}},
                        {RoomNum: "1003", Title: "西甲: 皇马 vs 巴萨", Cover: "https://cdn.zbyy.example/cover/1003.jpg", Live: false, ViewNum: 0, LiveType: "football", Anchor: findAnchor("A1003"), StreamUrls: []string{}, Notice: "比赛尚未开始", Tags: []string{"西甲", "国家德比"}},
                        {RoomNum: "1004", Title: "斯诺克世锦赛 半决赛", Cover: "https://cdn.zbyy.example/cover/1004.jpg", Live: false, ViewNum: 0, LiveType: "snooker", Anchor: findAnchor("A1004"), StreamUrls: []string{}, Notice: "静音观赛", Tags: []string{"斯诺克", "世锦赛"}},
                        {RoomNum: "1005", Title: "中超第20轮: 海港 vs 申花", Cover: "https://cdn.zbyy.example/cover/1005.jpg", Live: true, ViewNum: 19887, LiveType: "football", Anchor: findAnchor("A1005"), StreamUrls: []string{"https://live.zbyy.example/1005/hd.m3u8"}, Notice: "禁止地域攻击", Tags: []string{"中超", "上海德比"}},
                        {RoomNum: "1006", Title: "德甲: 拜仁 vs 多特", Cover: "https://cdn.zbyy.example/cover/1006.jpg", Live: false, ViewNum: 0, LiveType: "football", Anchor: findAnchor("A1006"), StreamUrls: []string{}, Notice: "德国国家德比", Tags: []string{"德甲", "拜仁", "多特"}},
                }

                lives = []LiveRoom{
                        {RoomNum: "1001", Title: "英超焦点战: 曼联 vs 利物浦", Cover: "https://cdn.zbyy.example/cover/1001.jpg", Anchor: findAnchor("A1001"), ViewNum: 38211, LiveType: "football", CateName: "英超"},
                        {RoomNum: "1002", Title: "NBA常规赛: 湖人 vs 勇士", Cover: "https://cdn.zbyy.example/cover/1002.jpg", Anchor: findAnchor("A1002"), ViewNum: 51209, LiveType: "basketball", CateName: "NBA"},
                        {RoomNum: "1005", Title: "中超第20轮: 海港 vs 申花", Cover: "https://cdn.zbyy.example/cover/1005.jpg", Anchor: findAnchor("A1005"), ViewNum: 19887, LiveType: "football", CateName: "中超"},
                }

                // Build matches for today + next 6 days
                matchesByDay = make(map[string][]MatchItem)
                now := time.Now()
                for d := 0; d < 7; d++ {
                        day := now.AddDate(0, 0, d)
                        dateKey := day.Format("20060102")
                        var list []MatchItem
                        list = append(list, MatchItem{
                                ScheduleId: dateKey + "01", SubCateName: "英超", CateName: "足球",
                                MatchTime: day.Add(20*time.Hour).UnixMilli(),
                                HostName: "曼联", HostIcon: "https://cdn.zbyy.example/team/man.png",
                                GuestName: "利物浦", GuestIcon: "https://cdn.zbyy.example/team/liv.png",
                                Venue: "老特拉福德", Status: matchStatus(day.Add(20 * time.Hour)),
                                Anchors: []Commentator{findAnchor("A1001"), findAnchor("A1006")},
                        })
                        list = append(list, MatchItem{
                                ScheduleId: dateKey + "02", SubCateName: "NBA", CateName: "篮球",
                                MatchTime: day.Add(11*time.Hour).UnixMilli(),
                                HostName: "湖人", HostIcon: "https://cdn.zbyy.example/team/lal.png",
                                GuestName: "勇士", GuestIcon: "https://cdn.zbyy.example/team/gsw.png",
                                Venue: "Crypto.com Arena", Status: matchStatus(day.Add(11 * time.Hour)),
                                Anchors: []Commentator{findAnchor("A1002")},
                        })
                        if d%2 == 0 {
                                list = append(list, MatchItem{
                                        ScheduleId: dateKey + "03", SubCateName: "西甲", CateName: "足球",
                                        MatchTime: day.Add(23 * time.Hour).UnixMilli(),
                                        HostName: "皇马", HostIcon: "https://cdn.zbyy.example/team/rma.png",
                                        GuestName: "巴萨", GuestIcon: "https://cdn.zbyy.example/team/bar.png",
                                        Venue: "伯纳乌", Status: matchStatus(day.Add(23 * time.Hour)),
                                        Anchors: []Commentator{findAnchor("A1003")},
                                })
                        }
                        if d == 1 {
                                list = append(list, MatchItem{
                                        ScheduleId: dateKey + "04", SubCateName: "德甲", CateName: "足球",
                                        MatchTime: day.Add(22 * time.Hour).UnixMilli(),
                                        HostName: "拜仁", HostIcon: "https://cdn.zbyy.example/team/bay.png",
                                        GuestName: "多特", GuestIcon: "https://cdn.zbyy.example/team/bvb.png",
                                        Venue: "安联球场", Status: matchStatus(day.Add(22 * time.Hour)),
                                        Anchors: []Commentator{findAnchor("A1006")},
                                })
                        }
                        matchesByDay[dateKey] = list
                        allMatches = append(allMatches, list...)
                }

                rankByRoom = map[string][]RoomRankItem{
                        "1001": {
                                {Uid: "U5001", NickName: "球迷老王", Icon: "https://cdn.zbyy.example/u/5001.png", Score: 18820, Rank: 1},
                                {Uid: "U5002", NickName: "红魔死忠", Icon: "https://cdn.zbyy.example/u/5002.png", Score: 12330, Rank: 2},
                                {Uid: "U5003", NickName: "安菲尔德之心", Icon: "https://cdn.zbyy.example/u/5003.png", Score: 9910, Rank: 3},
                        },
                        "1002": {
                                {Uid: "U6001", NickName: "紫金王朝", Icon: "https://cdn.zbyy.example/u/6001.png", Score: 22110, Rank: 1},
                                {Uid: "U6002", NickName: "萌神粉丝", Icon: "https://cdn.zbyy.example/u/6002.png", Score: 15020, Rank: 2},
                        },
                }
        })
}

func matchStatus(t time.Time) string {
        now := time.Now()
        if now.Before(t) {
                return "not_started"
        }
        if now.Sub(t) < 2*time.Hour+30*time.Minute {
                return "living"
        }
        return "over"
}

// ---------- Public loaders ----------

func Commentators() []Commentator      { seed(); mu.RLock(); defer mu.RUnlock(); out := make([]Commentator, len(anchors)); copy(out, anchors); return out }
func Rooms() []RoomDetail              { seed(); mu.RLock(); defer mu.RUnlock(); out := make([]RoomDetail, len(rooms)); copy(out, rooms); return out }
func Lives() []LiveRoom                { seed(); mu.RLock(); defer mu.RUnlock(); out := make([]LiveRoom, len(lives)); copy(out, lives); return out }
func LiveTypes() []LiveType            { seed(); mu.RLock(); defer mu.RUnlock(); out := make([]LiveType, len(liveTypes)); copy(out, liveTypes); return out }
func MatchesByDate(date string) []MatchItem { seed(); mu.RLock(); defer mu.RUnlock(); src := matchesByDay[date]; out := make([]MatchItem, len(src)); copy(out, src); return out }
func AllMatches() []MatchItem          { seed(); mu.RLock(); defer mu.RUnlock(); out := make([]MatchItem, len(allMatches)); copy(out, allMatches); return out }
func RankByRoom(roomNum string) []RoomRankItem { seed(); mu.RLock(); defer mu.RUnlock(); src := rankByRoom[roomNum]; out := make([]RoomRankItem, len(src)); copy(out, src); return out }

func CommentatorByID(uid string) (Commentator, bool) {
        seed(); mu.RLock(); defer mu.RUnlock()
        for _, a := range anchors {
                if a.Uid == uid {
                        return a, true
                }
        }
        return Commentator{}, false
}

func RoomByNum(roomNum string) (RoomDetail, bool) {
        seed(); mu.RLock(); defer mu.RUnlock()
        for _, r := range rooms {
                if r.RoomNum == roomNum {
                        return r, true
                }
        }
        return RoomDetail{}, false
}

func MatchByID(id string) (MatchItem, bool) {
        seed(); mu.RLock(); defer mu.RUnlock()
        for _, m := range allMatches {
                if m.ScheduleId == id {
                        return m, true
                }
        }
        return MatchItem{}, false
}

func CateNames() []string {
        seed(); mu.RLock(); defer mu.RUnlock()
        seen := map[string]bool{}
        var out []string
        for _, m := range allMatches {
                if !seen[m.CateName] {
                        seen[m.CateName] = true
                        out = append(out, m.CateName)
                }
        }
        return out
}

func MatchesByCate(cate string) []MatchItem {
        seed(); mu.RLock(); defer mu.RUnlock()
        var out []MatchItem
        for _, m := range allMatches {
                if m.CateName == cate {
                        out = append(out, m)
                }
        }
        return out
}

// Today returns today's date key YYYYMMDD
func Today() string { return time.Now().Format("20060102") }

// NowUnix returns current unix seconds
func NowUnix() int64 { return time.Now().Unix() }

// Recommend returns 5 hot upcoming/live matches
func Recommend() []MatchItem {
        seed(); mu.RLock(); defer mu.RUnlock()
        var out []MatchItem
        for _, m := range allMatches {
                if m.Status == "living" || m.Status == "not_started" {
                        out = append(out, m)
                        if len(out) >= 5 {
                                break
                        }
                }
        }
        return out
}

// HotAnchors returns commentators sorted by hotness, top N
func HotAnchors(n int) []Commentator {
        seed(); mu.RLock(); defer mu.RUnlock()
        out := make([]Commentator, len(anchors))
        copy(out, anchors)
        // simple insertion sort by Hot desc
        for i := 1; i < len(out); i++ {
                for j := i; j > 0 && out[j].Hot > out[j-1].Hot; j-- {
                        out[j], out[j-1] = out[j-1], out[j]
                }
        }
        if n > len(out) {
                n = len(out)
        }
        return out[:n]
}

// GenUid generates a new uid
func GenUid(prefix string) string {
        return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())
}

// ParsePage normalizes page/pageSize
func ParsePage(page, pageSize int32) (int, int) {
        if page < 1 {
                page = 1
        }
        if pageSize < 1 || pageSize > 100 {
                pageSize = 20
        }
        return int(page), int(pageSize)
}

// DateIsValid checks YYYYMMDD
func DateIsValid(date string) bool {
        if len(date) != 8 {
                return false
        }
        if _, err := time.Parse("20060102", date); err != nil {
                return false
        }
        return true
}

// Itoa safe int -> string
func Itoa(i int) string { return strconv.Itoa(i) }
