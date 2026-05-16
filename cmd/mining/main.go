package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/xpzouying/xiaohongshu-mcp/mining"
)

func main() {
	var (
		keywordsPath = flag.String("keywords", "configs/mining_keywords.yml", "种子词配置文件路径")
		dbPath       = flag.String("db", "data/mining.db", "SQLite 数据库路径")
		dumpJSONDir  = flag.String("dump-json", "", "可选: 把每条标准化笔记导出到该目录(便于人工 review)")
		maxPerKW     = flag.Int("max-per-keyword", 20, "每个搜索词最多抓取多少条笔记")
	)
	flag.Parse()

	cfg, err := mining.LoadKeywords(*keywordsPath)
	if err != nil {
		logrus.Fatalf("加载种子词失败: %v", err)
	}
	if err := os.MkdirAll("data", 0o755); err != nil {
		logrus.Fatalf("创建 data 目录失败: %v", err)
	}

	store, err := mining.OpenStore(*dbPath)
	if err != nil {
		logrus.Fatalf("打开数据库失败: %v", err)
	}
	defer store.Close()

	var sinks []mining.NoteSink
	if *dumpJSONDir != "" {
		dump, err := mining.NewJSONDumpSink(*dumpJSONDir)
		if err != nil {
			logrus.Fatalf("初始化 JSON dump 失败: %v", err)
		}
		sinks = append(sinks, dump)
		logrus.Infof("已开启 JSON dump: %s", *dumpJSONDir)
	}

	opts := mining.DefaultOptions()
	opts.MaxFeedsPerKeyword = *maxPerKW

	p := &mining.Pipeline{
		Fetcher:  newBrowserFetcher(),
		Store:    store,
		Sinks:    sinks,
		Keywords: cfg,
		Opts:     opts,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	sum, err := p.Run(ctx)
	logrus.Infof("汇总: 搜索词=%d 候选笔记=%d 预过滤淘汰=%d 增量跳过=%d 详情失败=%d 入库=%d",
		sum.SearchedKeywords, sum.TotalFeeds, sum.PreFilteredOut,
		sum.SkippedRecent, sum.DetailFailed, sum.Saved)
	if err != nil {
		logrus.Errorf("流水线异常退出: %v", err)
		os.Exit(1)
	}
}
