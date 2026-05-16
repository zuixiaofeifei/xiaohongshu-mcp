package mining

import "time"

// Note 是流水线的核心产物: LLM-Ready 的标准化笔记结构。
// 一条 Note 包含分析所需的全部上下文(内容/指标/作者/交易信号/精选评论),
// 后续 LLM 分析直接消费这个结构,无需再回查原始接口。
type Note struct {
	NoteID       string    `json:"note_id"`
	URL          string    `json:"url"`
	CategorySeed string    `json:"category_seed"` // 命中哪个种子分类
	FetchedAt    time.Time `json:"fetched_at"`

	Title       string `json:"title"`
	Content     string `json:"content"`
	PublishTime string `json:"publish_time,omitempty"`

	Metrics Metrics `json:"metrics"`
	Author  Author  `json:"author"`

	TradeSignals TradeSignals `json:"trade_signals"`
	KeyComments  []Comment    `json:"key_comments"`
}

// Metrics 互动指标(已解析为数字)
type Metrics struct {
	Likes    int `json:"likes"`
	Comments int `json:"comments"`
	Collects int `json:"collects"`
}

// Author 作者信息(精简版)
type Author struct {
	UserID   string `json:"user_id"`
	Nickname string `json:"nickname"`
	IP       string `json:"ip,omitempty"`
}

// TradeSignals 规则提取的交易信号,用于打分和提示 LLM。
// SignalScore = 命中类别数(0-3),阈值用于过滤噪音。
type TradeSignals struct {
	PriceHits   []string `json:"price_hits"`   // 价格命中: ¥9.9 / 29元
	ContactHits []string `json:"contact_hits"` // 联系方式信号: 私我/主页
	ProductHits []string `json:"product_hits"` // 商品类型词: 教程/合集/指令
	SignalScore int      `json:"signal_score"`
}

// Comment 精选评论(已经按规则筛选过)
type Comment struct {
	ByAuthor bool   `json:"by_author"` // 是否楼主回复
	Content  string `json:"content"`
	Likes    int    `json:"likes"`
}
