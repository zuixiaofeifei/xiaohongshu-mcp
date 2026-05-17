package main

import (
	"context"
	"fmt"
	"time"

	"github.com/xpzouying/xiaohongshu-mcp/browser"
	"github.com/xpzouying/xiaohongshu-mcp/configs"
	"github.com/xpzouying/xiaohongshu-mcp/xiaohongshu"
)

// 单次抓取的硬超时,避免单条坏 feed 拖死整条流水线。
// 项目原生评论加载逻辑默认重试 500 次,无超时时最长约 75 分钟。
const (
	searchTimeout = 60 * time.Second // 实测 30s 偶尔不够(SSR 渲染慢/网络抖动)
	detailTimeout = 90 * time.Second
)

// browserFetcher 用项目现有 headless_browser 实现 mining.XHSFetcher 接口。
// 每次调用新建独立浏览器,与 main 包的 service 层行为一致(反爬考虑)。
type browserFetcher struct{}

func newBrowserFetcher() *browserFetcher { return &browserFetcher{} }

// Search 关键词搜索
func (f *browserFetcher) Search(ctx context.Context, keyword string) (feeds []xiaohongshu.Feed, err error) {
	defer recoverAsError(&err)

	ctx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()

	b := browser.NewBrowser(configs.IsHeadless(), browser.WithBinPath(configs.GetBinPath()))
	defer b.Close()

	page := b.NewPage()
	defer page.Close()

	return xiaohongshu.NewSearchAction(page).Search(ctx, keyword)
}

// FeedDetail 详情(含全量评论)
func (f *browserFetcher) FeedDetail(ctx context.Context, feedID, xsecToken string) (resp *xiaohongshu.FeedDetailResponse, err error) {
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

// recoverAsError 把 rod 的 panic(尤其是 ctx 超时引起的 MustXxx panic) 转成 error 返回。
// 这样单条 feed 抓取失败不会拖垮整条流水线。
func recoverAsError(out *error) {
	if r := recover(); r != nil {
		if e, ok := r.(error); ok {
			*out = fmt.Errorf("浏览器操作 panic: %w", e)
		} else {
			*out = fmt.Errorf("浏览器操作 panic: %v", r)
		}
	}
}
