package mining

import (
	"context"
	"math/rand"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/xpzouying/xiaohongshu-mcp/xiaohongshu"
)

// XHSFetcher 抓取层依赖,由调用方注入。
// 这样 mining 包不直接持有浏览器逻辑,方便后续替换/测试。
type XHSFetcher interface {
	Search(ctx context.Context, keyword string) ([]xiaohongshu.Feed, error)
	FeedDetail(ctx context.Context, feedID, xsecToken string) (*xiaohongshu.FeedDetailResponse, error)
}

// NoteSink 标准化笔记的接收器(可同时落库 + 导出 JSON 等)
type NoteSink interface {
	SaveNote(n *Note) error
}

// Pipeline 数据抓取流水线: keyword → search → 预过滤 → detail → normalize → sink
type Pipeline struct {
	Fetcher  XHSFetcher
	Store    *Store
	Sinks    []NoteSink // 附加输出(如 JSON dump),Store 不在内
	Keywords *KeywordsConfig
	Opts     PipelineOptions
}

// PipelineOptions 运行参数
type PipelineOptions struct {
	MaxFeedsPerKeyword int           // 每个搜索词最多保留多少条搜索结果
	RecentSkipDuration time.Duration // 已抓过的笔记多久内不重抓
	SearchInterval     [2]int        // 搜索间隔随机区间(秒)
	DetailInterval     [2]int        // 详情间隔随机区间(秒)
}

// DefaultOptions 默认运行参数
func DefaultOptions() PipelineOptions {
	return PipelineOptions{
		MaxFeedsPerKeyword: 20,
		RecentSkipDuration: 7 * 24 * time.Hour,
		SearchInterval:     [2]int{5, 15},
		DetailInterval:     [2]int{3, 10},
	}
}

// Summary 一次运行的汇总统计
type Summary struct {
	SearchedKeywords int
	TotalFeeds       int
	PreFilteredOut   int
	SkippedRecent    int
	DetailFailed     int
	Saved            int
}

func (s *Summary) merge(o Summary) {
	s.SearchedKeywords += o.SearchedKeywords
	s.TotalFeeds += o.TotalFeeds
	s.PreFilteredOut += o.PreFilteredOut
	s.SkippedRecent += o.SkippedRecent
	s.DetailFailed += o.DetailFailed
	s.Saved += o.Saved
}

// Run 执行完整流水线: 遍历所有 SearchQuery → 落库
func (p *Pipeline) Run(ctx context.Context) (Summary, error) {
	queries := p.Keywords.BuildSearchQueries()
	logrus.Infof("生成搜索任务 %d 条", len(queries))

	var sum Summary
	for i, q := range queries {
		if err := ctx.Err(); err != nil {
			return sum, err
		}
		logrus.Infof("[%d/%d] [%s] 搜索: %s", i+1, len(queries), q.Category, q.Keyword)
		stats, err := p.runOne(ctx, q)
		if err != nil {
			logrus.Errorf("搜索 %q 失败: %v", q.Keyword, err)
		}
		sum.merge(stats)
		if i < len(queries)-1 {
			sleepRandom(ctx, p.Opts.SearchInterval)
		}
	}
	return sum, nil
}

// runOne 处理单条搜索任务
func (p *Pipeline) runOne(ctx context.Context, q SearchQuery) (Summary, error) {
	var sum Summary
	sum.SearchedKeywords = 1

	feeds, err := p.Fetcher.Search(ctx, q.Keyword)
	if err != nil {
		return sum, err
	}
	if len(feeds) > p.Opts.MaxFeedsPerKeyword {
		feeds = feeds[:p.Opts.MaxFeedsPerKeyword]
	}

	for _, feed := range feeds {
		sum.TotalFeeds++

		// 预过滤: 只用搜索结果的 displayTitle 判断,避免抓不必要的详情
		if !PassPreFilter(feed.NoteCard.DisplayTitle, "") {
			sum.PreFilteredOut++
			continue
		}

		// 增量去重
		if p.Store != nil {
			recent, err := p.Store.NoteFetchedWithin(feed.ID, p.Opts.RecentSkipDuration)
			if err != nil {
				logrus.Warnf("查询 note 状态失败 %s: %v", feed.ID, err)
			}
			if recent {
				sum.SkippedRecent++
				continue
			}
		}

		// 详情(含全量评论)
		detail, err := p.Fetcher.FeedDetail(ctx, feed.ID, feed.XsecToken)
		if err != nil {
			logrus.Warnf("详情失败 %s: %v", feed.ID, err)
			sum.DetailFailed++
			continue
		}

		// 标准化
		note := Normalize(q.Category, detail)

		// 入库
		if p.Store != nil {
			if err := p.Store.SaveNote(note); err != nil {
				logrus.Warnf("入库失败 %s: %v", note.NoteID, err)
				continue
			}
		}
		// 附加 sink(如 JSON dump)
		for _, sink := range p.Sinks {
			if err := sink.SaveNote(note); err != nil {
				logrus.Warnf("sink 写入失败 %s: %v", note.NoteID, err)
			}
		}
		sum.Saved++
		sleepRandom(ctx, p.Opts.DetailInterval)
	}
	return sum, nil
}

// sleepRandom 在随机区间内等待,但响应 ctx 取消,Ctrl+C 立刻退出
func sleepRandom(ctx context.Context, rng [2]int) {
	min, max := rng[0], rng[1]
	if max <= min {
		max = min + 1
	}
	d := time.Duration(min+rand.Intn(max-min)) * time.Second
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}
