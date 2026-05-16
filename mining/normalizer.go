package mining

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/xpzouying/xiaohongshu-mcp/xiaohongshu"
)

// 评论筛选参数
const (
	maxKeyComments    = 10   // 最多保留多少条评论喂 LLM
	totalCommentBytes = 3000 // 全部 key_comments 内容长度上限
)

// Normalize 把原生 FeedDetailResponse 转换为 LLM-Ready 的 Note。
// 流程: 抽取信号 → 评论筛选 → 二次补信号 → 组装结构。
func Normalize(category string, detail *xiaohongshu.FeedDetailResponse) *Note {
	note := detail.Note

	signals := ExtractSignals(note.Title + " " + note.Desc)
	keyComments := selectKeyComments(note.User.UserID, detail.Comments.List)
	enrichSignalsFromComments(&signals, keyComments)

	return &Note{
		NoteID:       note.NoteID,
		URL:          "https://www.xiaohongshu.com/explore/" + note.NoteID,
		CategorySeed: category,
		FetchedAt:    time.Now(),
		Title:        note.Title,
		Content:      note.Desc,
		PublishTime:  formatTime(note.Time),
		Metrics: Metrics{
			Likes:    parseCount(note.InteractInfo.LikedCount),
			Comments: parseCount(note.InteractInfo.CommentCount),
			Collects: parseCount(note.InteractInfo.CollectedCount),
		},
		Author: Author{
			UserID:   note.User.UserID,
			Nickname: firstNonEmpty(note.User.Nickname, note.User.NickName),
			IP:       note.IPLocation,
		},
		TradeSignals: signals,
		KeyComments:  keyComments,
	}
}

// selectKeyComments 评论筛选策略:
// 1. 楼主回复优先(揭示交付方式 = 高价值)
// 2. 含交易信号的评论
// 3. 高赞评论
// 综合打分排序,按 totalCommentBytes 截断。
func selectKeyComments(authorID string, comments []xiaohongshu.Comment) []Comment {
	type scored struct {
		c     xiaohongshu.Comment
		score int
	}
	pool := make([]scored, 0, len(comments))
	for _, c := range comments {
		s := 0
		if c.UserInfo.UserID == authorID {
			s += 100
		}
		if ExtractSignals(c.Content).SignalScore > 0 {
			s += 10
		}
		s += parseCount(c.LikeCount)
		pool = append(pool, scored{c, s})
	}
	sort.SliceStable(pool, func(i, j int) bool { return pool[i].score > pool[j].score })

	result := make([]Comment, 0, maxKeyComments)
	used := 0
	for _, s := range pool {
		if len(result) >= maxKeyComments {
			break
		}
		if used+len(s.c.Content) > totalCommentBytes {
			break
		}
		result = append(result, Comment{
			ByAuthor: s.c.UserInfo.UserID == authorID,
			Content:  s.c.Content,
			Likes:    parseCount(s.c.LikeCount),
		})
		used += len(s.c.Content)
	}
	return result
}

// enrichSignalsFromComments 把评论里出现的信号也累加到笔记信号上
func enrichSignalsFromComments(signals *TradeSignals, comments []Comment) {
	for _, c := range comments {
		s := ExtractSignals(c.Content)
		signals.PriceHits = append(signals.PriceHits, s.PriceHits...)
		signals.ContactHits = append(signals.ContactHits, s.ContactHits...)
		signals.ProductHits = append(signals.ProductHits, s.ProductHits...)
	}
	signals.SignalScore = signalScore(*signals)
}

// parseCount 把 "1.2万" / "1234" / "" 转成 int
func parseCount(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if strings.HasSuffix(s, "万") {
		f, err := strconv.ParseFloat(strings.TrimSuffix(s, "万"), 64)
		if err != nil {
			return 0
		}
		return int(f * 10000)
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// formatTime 毫秒时间戳 → YYYY-MM-DD
func formatTime(ts int64) string {
	if ts <= 0 {
		return ""
	}
	return time.Unix(ts/1000, 0).Format("2006-01-02")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
