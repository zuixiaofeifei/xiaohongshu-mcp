// cmd/explore: 发现型抓取,不依赖关键词,从小红书首页推荐流捞数据。
//
// 用关键词搜索 = "我知道要找什么",视野受限于种子词。
// 用首页推荐流 = "看看算法在推什么",发现意料之外的需求方向。
//
// 用法:
//
//	go run ./cmd/explore -rounds 10 -max-per-round 20
//
// 抓到的笔记 category_seed 标记为 "explore-home"。
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/xpzouying/xiaohongshu-mcp/browser"
	"github.com/xpzouying/xiaohongshu-mcp/configs"
	"github.com/xpzouying/xiaohongshu-mcp/mining"
	"github.com/xpzouying/xiaohongshu-mcp/xiaohongshu"
)

const (
	feedsTimeout  = 60 * time.Second
	detailTimeout = 90 * time.Second
)

func main() {
	var (
		dbPath      = flag.String("db", "data/mining.db", "SQLite 数据库路径")
		dumpJSONDir = flag.String("dump-json", "", "可选: 笔记 JSON 导出目录")
		rounds      = flag.Int("rounds", 5, "抓取轮数(每轮拉一批首页推荐)")
		maxPerRound = flag.Int("max-per-round", 15, "每轮最多处理多少条候选笔记")
		roundGapMin = flag.Int("round-gap-min", 30, "轮次间隔最小秒数")
		roundGapMax = flag.Int("round-gap-max", 90, "轮次间隔最大秒数")
		recentDays  = flag.Int("recent-days", 7, "最近 N 天内抓过的笔记跳过")
	)
	flag.Parse()

	if err := os.MkdirAll(filepath.Dir(*dbPath), 0o755); err != nil {
		logrus.Fatalf("创建 data 目录失败: %v", err)
	}

	store, err := mining.OpenStore(*dbPath)
	if err != nil {
		logrus.Fatalf("打开数据库失败: %v", err)
	}
	defer store.Close()

	var dump *mining.JSONDumpSink
	if *dumpJSONDir != "" {
		dump, err = mining.NewJSONDumpSink(*dumpJSONDir)
		if err != nil {
			logrus.Fatalf("初始化 JSON dump 失败: %v", err)
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	recentSkip := time.Duration(*recentDays) * 24 * time.Hour
	totalSaved := 0

	for round := 1; round <= *rounds; round++ {
		if err := ctx.Err(); err != nil {
			break
		}
		logrus.Infof("[%d/%d] 拉取首页推荐...", round, *rounds)

		feeds, err := fetchHomeFeeds(ctx)
		if err != nil {
			logrus.Errorf("拉取首页失败: %v", err)
			continue
		}
		logrus.Infof("  ✓ 拿到 %d 条候选", len(feeds))

		if len(feeds) > *maxPerRound {
			feeds = feeds[:*maxPerRound]
		}
		saved := processFeeds(ctx, feeds, store, dump, recentSkip)
		totalSaved += saved
		logrus.Infof("  本轮入库: %d / 累计: %d", saved, totalSaved)

		if round < *rounds {
			sleepRandomCtx(ctx, *roundGapMin, *roundGapMax)
		}
	}
	logrus.Infof("完成: 共 %d 轮, 入库 %d 条", *rounds, totalSaved)
}

// fetchHomeFeeds 调一次首页 feeds 接口
func fetchHomeFeeds(ctx context.Context) (feeds []xiaohongshu.Feed, err error) {
	defer recoverAsError(&err)

	ctx, cancel := context.WithTimeout(ctx, feedsTimeout)
	defer cancel()

	b := browser.NewBrowser(configs.IsHeadless(), browser.WithBinPath(configs.GetBinPath()))
	defer b.Close()
	page := b.NewPage()
	defer page.Close()

	return xiaohongshu.NewFeedsListAction(page).GetFeedsList(ctx)
}

// processFeeds 对每条候选笔记: 预过滤 → 去重 → 抓详情 → 标准化 → 入库
func processFeeds(ctx context.Context, feeds []xiaohongshu.Feed, store *mining.Store, dump *mining.JSONDumpSink, recentSkip time.Duration) int {
	saved := 0
	for _, feed := range feeds {
		if err := ctx.Err(); err != nil {
			return saved
		}

		// 预过滤: 首页推荐普遍信号弱,这里也走规则筛掉明显噪音
		if !mining.PassPreFilter(feed.NoteCard.DisplayTitle, "") {
			continue
		}

		// 增量去重
		recent, err := store.NoteFetchedWithin(feed.ID, recentSkip)
		if err != nil {
			logrus.Warnf("查询去重状态失败 %s: %v", feed.ID, err)
		}
		if recent {
			continue
		}

		// 详情(带超时,避免坏 feed 拖死)
		detail, err := fetchDetail(ctx, feed.ID, feed.XsecToken)
		if err != nil {
			logrus.Warnf("详情失败 %s: %v", feed.ID, err)
			continue
		}

		note := mining.Normalize("explore-home", detail)
		if err := store.SaveNote(note); err != nil {
			logrus.Warnf("入库失败 %s: %v", note.NoteID, err)
			continue
		}
		if dump != nil {
			_ = dump.SaveNote(note)
		}
		saved++

		// 详情间隔
		sleepRandomCtx(ctx, 3, 10)
	}
	return saved
}

func fetchDetail(ctx context.Context, feedID, xsecToken string) (resp *xiaohongshu.FeedDetailResponse, err error) {
	defer recoverAsError(&err)

	ctx, cancel := context.WithTimeout(ctx, detailTimeout)
	defer cancel()

	b := browser.NewBrowser(configs.IsHeadless(), browser.WithBinPath(configs.GetBinPath()))
	defer b.Close()
	page := b.NewPage()
	defer page.Close()

	return xiaohongshu.NewFeedDetailAction(page).
		GetFeedDetail(ctx, feedID, xsecToken, true, xiaohongshu.DefaultCommentLoadConfig())
}

// recoverAsError 把 rod 的 panic 转 error
func recoverAsError(out *error) {
	if r := recover(); r != nil {
		if e, ok := r.(error); ok {
			*out = fmt.Errorf("浏览器操作 panic: %w", e)
		} else {
			*out = fmt.Errorf("浏览器操作 panic: %v", r)
		}
	}
}

func sleepRandomCtx(ctx context.Context, min, max int) {
	if max <= min {
		max = min + 1
	}
	d := time.Duration(min+rand.Intn(max-min)) * time.Second
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}
