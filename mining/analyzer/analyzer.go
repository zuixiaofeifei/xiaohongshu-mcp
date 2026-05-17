package analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/sirupsen/logrus"
	"github.com/xpzouying/xiaohongshu-mcp/mining"
)

// Analyzer 把 mining.Note 喂给 LLM,得到 Opportunity
type Analyzer struct {
	client *Client
}

// New 构造
func New(client *Client) *Analyzer {
	return &Analyzer{client: client}
}

// Analyze 对单条 Note 跑 LLM 分析,返回 Opportunity + 使用的 token 数
func (a *Analyzer) Analyze(ctx context.Context, n *mining.Note) (*Opportunity, int, error) {
	userPrompt, err := BuildUserPrompt(n)
	if err != nil {
		return nil, 0, err
	}

	var (
		opp    *Opportunity
		tokens int
	)
	err = retry.Do(
		func() error {
			raw, t, err := a.client.ChatJSON(ctx, SystemPrompt(), userPrompt)
			if err != nil {
				return err
			}
			tokens = t
			var o Opportunity
			if err := json.Unmarshal([]byte(raw), &o); err != nil {
				return fmt.Errorf("LLM 返回非合法 JSON: %w, raw=%s", err, truncate(raw, 200))
			}
			opp = &o
			return nil
		},
		retry.Attempts(2),
		retry.Delay(2*time.Second),
		retry.OnRetry(func(n uint, err error) {
			logrus.Warnf("analyze 重试 #%d: %v", n+1, err)
		}),
		retry.Context(ctx),
	)
	if err != nil {
		return nil, tokens, err
	}
	return opp, tokens, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
