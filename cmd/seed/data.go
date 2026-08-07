package main

// Seed data — mirrors the zbyy fixture data.
// Each row is a []interface{} matching the column order.

var liveTypes = [][]string{
	{"football", "足球", "https://cdn.zbyy.example/ico/football.png", "1"},
	{"basketball", "篮球", "https://cdn.zbyy.example/ico/basketball.png", "2"},
	{"snooker", "斯诺克", "https://cdn.zbyy.example/ico/snooker.png", "3"},
	{"other", "其它", "https://cdn.zbyy.example/ico/other.png", "4"},
}

// anchors: uid, nick, icon, cutout, intro, fans, follow, hot, room, detail, notice, live
var anchors = [][]string{
	{"A1001", "飞鱼解说", "https://cdn.zbyy.example/avatar/a1001.png", "https://cdn.zbyy.example/avatar/a1001_cut.png",
		"前职业球员，专注英超解说10年", "128000", "98000", "9527", "1001", "每晚8点英超直播", "禁止刷屏、禁止广告", "1"},
	{"A1002", "篮球小子", "https://cdn.zbyy.example/avatar/a1002.png", "https://cdn.zbyy.example/avatar/a1002_cut.png",
		"NBA/CBA深度分析", "86000", "64000", "7610", "1002", "NBA专场", "文明观赛", "1"},
	{"A1003", "绿茵观察", "https://cdn.zbyy.example/avatar/a1003.png", "https://cdn.zbyy.example/avatar/a1003_cut.png",
		"西甲、欧冠战术分析", "54000", "41000", "5230", "1003", "西甲之夜", "理性讨论", "0"},
	{"A1004", "斯诺克达人", "https://cdn.zbyy.example/avatar/a1004.png", "https://cdn.zbyy.example/avatar/a1004_cut.png",
		"斯诺克职业赛事解说", "21000", "18000", "2100", "1004", "斯诺克直播", "安静观赛", "0"},
	{"A1005", "中超前线", "https://cdn.zbyy.example/avatar/a1005.png", "https://cdn.zbyy.example/avatar/a1005_cut.png",
		"中超、亚冠现场报道", "39000", "30000", "3340", "1005", "中超集锦", "禁止地域攻击", "1"},
	{"A1006", "德甲工匠", "https://cdn.zbyy.example/avatar/a1006.png", "https://cdn.zbyy.example/avatar/a1006_cut.png",
		"德甲战术拆解", "47000", "35000", "4120", "1006", "德甲周末", "文明互动", "0"},
}

// rooms: room_num, title, cover, live, view_num, live_type, anchor_uid, stream_urls[], notice, tags[], cate_name
var rooms = [][]interface{}{
	{"1001", "英超焦点战: 曼联 vs 利物浦", "https://cdn.zbyy.example/cover/1001.jpg", "1", "38211", "football", "A1001",
		[]string{"https://live.zbyy.example/1001/hd.m3u8", "https://live.zbyy.example/1001/sd.m3u8"},
		"文明观赛，禁止刷屏",
		[]string{"英超", "曼联", "利物浦"}, "英超"},
	{"1002", "NBA常规赛: 湖人 vs 勇士", "https://cdn.zbyy.example/cover/1002.jpg", "1", "51209", "basketball", "A1002",
		[]string{"https://live.zbyy.example/1002/hd.m3u8"},
		"理性讨论",
		[]string{"NBA", "湖人", "勇士"}, "NBA"},
	{"1003", "西甲: 皇马 vs 巴萨", "https://cdn.zbyy.example/cover/1003.jpg", "0", "0", "football", "A1003",
		[]string{},
		"比赛尚未开始",
		[]string{"西甲", "国家德比"}, "西甲"},
	{"1004", "斯诺克世锦赛 半决赛", "https://cdn.zbyy.example/cover/1004.jpg", "0", "0", "snooker", "A1004",
		[]string{},
		"静音观赛",
		[]string{"斯诺克", "世锦赛"}, "斯诺克"},
	{"1005", "中超第20轮: 海港 vs 申花", "https://cdn.zbyy.example/cover/1005.jpg", "1", "19887", "football", "A1005",
		[]string{"https://live.zbyy.example/1005/hd.m3u8"},
		"禁止地域攻击",
		[]string{"中超", "上海德比"}, "中超"},
	{"1006", "德甲: 拜仁 vs 多特", "https://cdn.zbyy.example/cover/1006.jpg", "0", "0", "football", "A1006",
		[]string{},
		"德国国家德比",
		[]string{"德甲", "拜仁", "多特"}, "德甲"},
}

// room_ranks: room_num, uid, nick, icon, score, rank_no
var roomRanks = [][]string{
	{"1001", "U5001", "球迷老王", "https://cdn.zbyy.example/u/5001.png", "18820", "1"},
	{"1001", "U5002", "红魔死忠", "https://cdn.zbyy.example/u/5002.png", "12330", "2"},
	{"1001", "U5003", "安菲尔德之心", "https://cdn.zbyy.example/u/5003.png", "9910", "3"},
	{"1002", "U6001", "紫金王朝", "https://cdn.zbyy.example/u/6001.png", "22110", "1"},
	{"1002", "U6002", "萌神粉丝", "https://cdn.zbyy.example/u/6002.png", "15020", "2"},
}
