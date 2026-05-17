// cmd/analyze: 把 SQLite 里的笔记批量喂给 DeepSeek,产出机会判断。
// 用法:
//
//	go run ./cmd/analyze -db data/mining.db -min-signal 1 -limit 10
//
// 同一条 note 在相同 llm_version 下不会重复跑(由 store 层过滤)。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/xpzouying/xiaohongshu-mcp/mining"
	"github.com/xpzouying/xiaohongshu-mcp/mining/analyzer"
)

func main() {
	var (
		dbPath     = flag.String("db", "data/mining.db", "SQLite 数据库路径")
		cfgPath    = flag.String("config", "configs/deepseek.yml", "DeepSeek 配置文件")
		minSignal  = flag.Int("min-signal", 1, "只分析 signal_score >= 此值的笔记")
		limit      = flag.Int("limit", 20, "本次最多分析多少条")
		llmVersion = flag.String("llm-version", analyzer.PromptVersion, "LLM 版本号(prompt 变更时 bump)")
	)
	flag.Parse()

	cfg, err := analyzer.LoadConfig(*cfgPath)
	if err != nil {
		logrus.Fatalf("加载 DeepSeek 配置失败: %v", err)
	}
	logrus.Infof("使用模型: %s, endpoint: %s", cfg.Model, cfg.Endpoint)

	store, err := mining.OpenStore(*dbPath)
	if err != nil {
		logrus.Fatalf("打开数据库失败: %v", err)
	}
	defer store.Close()

	notes, err := store.ListNotesNeedAnalysis(*minSignal, *llmVersion, *limit)
	if err != nil {
		logrus.Fatalf("查询待分析笔记失败: %v", err)
	}
	if len(notes) == 0 {
		logrus.Info("没有需要分析的笔记(可能都已分析过当前 llm_version)")
		return
	}
	logrus.Infof("准备分析 %d 条笔记(llm_version=%s)", len(notes), *llmVersion)

	a := analyzer.New(analyzer.NewClient(cfg))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var (
		success    int
		failed     int
		totalToken int
	)
	for i, n := range notes {
		if err := ctx.Err(); err != nil {
			logrus.Warn("收到中断信号,提前退出")
			break
		}
		logrus.Infof("[%d/%d] 分析: %s", i+1, len(notes), n.Title)
		opp, tokens, err := a.Analyze(ctx, n)
		totalToken += tokens
		if err != nil {
			logrus.Errorf("分析失败 %s: %v", n.NoteID, err)
			failed++
			continue
		}
		oppJSON, _ := json.Marshal(opp)
		if err := store.SaveAnalysis(n.NoteID, *llmVersion, string(oppJSON), opp.CompositeScore()); err != nil {
			logrus.Errorf("入库失败 %s: %v", n.NoteID, err)
			failed++
			continue
		}
		success++
		logrus.Infof("  ✓ verdict=%s score=%d tokens=%d", opp.Verdict, opp.CompositeScore(), tokens)
	}

	logrus.Infof("完成: 成功=%d 失败=%d 总 token=%d", success, failed, totalToken)
}
