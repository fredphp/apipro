package conv

// Conversions between go-zero generated api types (JSON) and rpc proto types.

import (
        "apipro/cmd/api/internal/types"
        "apipro/cmd/rpc/apiproClient"
)

func Anchor(p *apiproClient.Anchor) types.AnchorJson {
        if p == nil {
                return types.AnchorJson{}
        }
        return types.AnchorJson{Uid: p.Uid, RoomNum: p.RoomNum, Detail: p.Detail, Notice: p.Notice, Live: p.Live}
}

func Commentator(p *apiproClient.Commentator) types.CommentatorJson {
        if p == nil {
                return types.CommentatorJson{}
        }
        return types.CommentatorJson{
                Uid: p.Uid, NickName: p.NickName, Icon: p.Icon, CutOutIcon: p.CutOutIcon,
                Intro: p.Intro, Fans: p.Fans, Follow: p.Follow, Hot: p.Hot, Anchor: Anchor(p.Anchor),
        }
}

func Commentators(p []*apiproClient.Commentator) []types.CommentatorJson {
        out := make([]types.CommentatorJson, 0, len(p))
        for _, c := range p {
                out = append(out, Commentator(c))
        }
        return out
}

func MatchItem(p *apiproClient.MatchItem) types.MatchItemJson {
        if p == nil {
                return types.MatchItemJson{}
        }
        return types.MatchItemJson{
                ScheduleId: p.ScheduleId, SubCateName: p.SubCateName, CateName: p.CateName,
                MatchTime: p.MatchTime, HostName: p.HostName, HostIcon: p.HostIcon,
                GuestName: p.GuestName, GuestIcon: p.GuestIcon, Venue: p.Venue,
                Status: p.Status, ReservationStatus: p.ReservationStatus,
                Anchors: Commentators(p.Anchors),
                CategoryId: p.CategoryId, CategoryName: p.CategoryName,
                CategoryIcon: p.CategoryIcon, MatchStatusDesc: p.MatchStatusDesc,
                HostScore: p.HostScore, GuestScore: p.GuestScore,
        }
}

func MatchItems(p []*apiproClient.MatchItem) []types.MatchItemJson {
        out := make([]types.MatchItemJson, 0, len(p))
        for _, m := range p {
                out = append(out, MatchItem(m))
        }
        return out
}

func RoomDetail(p *apiproClient.RoomDetail) types.RoomDetailJson {
        if p == nil {
                return types.RoomDetailJson{}
        }
        return types.RoomDetailJson{
                RoomNum: p.RoomNum, Title: p.Title, Cover: p.Cover, Live: p.Live,
                ViewNum: p.ViewNum, LiveType: p.LiveType, Anchor: Commentator(p.Anchor),
                StreamUrls: p.StreamUrls, Notice: p.Notice, Tags: p.Tags,
        }
}

func LiveRoom(p *apiproClient.LiveRoom) types.LiveRoomJson {
        if p == nil {
                return types.LiveRoomJson{}
        }
        return types.LiveRoomJson{
                RoomNum: p.RoomNum, Title: p.Title, Cover: p.Cover,
                Anchor: Commentator(p.Anchor), ViewNum: p.ViewNum,
                LiveType: p.LiveType, CateName: p.CateName,
                ViewCount: p.ViewCount, CutOutCustomCoverUrl: p.CutOutCustomCoverUrl,
                MarkType: p.MarkType, LiveStatus: p.LiveStatus,
        }
}

func LiveRooms(p []*apiproClient.LiveRoom) []types.LiveRoomJson {
        out := make([]types.LiveRoomJson, 0, len(p))
        for _, r := range p {
                out = append(out, LiveRoom(r))
        }
        return out
}

func LiveType(p *apiproClient.LiveType) types.LiveTypeJson {
        if p == nil {
                return types.LiveTypeJson{}
        }
        return types.LiveTypeJson{Code: p.Code, Name: p.Name, Icon: p.Icon}
}

func LiveTypes(p []*apiproClient.LiveType) []types.LiveTypeJson {
        out := make([]types.LiveTypeJson, 0, len(p))
        for _, t := range p {
                out = append(out, LiveType(t))
        }
        return out
}

func RoomRankItem(p *apiproClient.RoomRankItem) types.RoomRankItemJson {
        if p == nil {
                return types.RoomRankItemJson{}
        }
        return types.RoomRankItemJson{Uid: p.Uid, NickName: p.NickName, Icon: p.Icon, Score: p.Score, Rank: p.Rank}
}

func RoomRankItems(p []*apiproClient.RoomRankItem) []types.RoomRankItemJson {
        out := make([]types.RoomRankItemJson, 0, len(p))
        for _, r := range p {
                out = append(out, RoomRankItem(r))
        }
        return out
}

func FamilyStat(p *apiproClient.FamilyStat) types.FamilyStatJson {
        if p == nil {
                return types.FamilyStatJson{}
        }
        return types.FamilyStatJson{Family: p.Family, Keys: p.Keys, Hits: p.Hits, Misses: p.Misses}
}

func FamilyStats(p []*apiproClient.FamilyStat) []types.FamilyStatJson {
        out := make([]types.FamilyStatJson, 0, len(p))
        for _, f := range p {
                out = append(out, FamilyStat(f))
        }
        return out
}

func UserProfile(p *apiproClient.UserInfo) types.UserProfileResp {
        if p == nil {
                return types.UserProfileResp{}
        }
        return types.UserProfileResp{
                Uid: p.Uid, LoginName: p.LoginName, NickName: p.NickName,
                Phone: p.Phone, CountryCode: p.CountryCode, Grow: p.Grow, Score: p.Score,
                Level: p.Level, Avatar: p.Avatar, IsUser: p.IsUser, CreatedAt: p.CreatedAt,
        }
}
