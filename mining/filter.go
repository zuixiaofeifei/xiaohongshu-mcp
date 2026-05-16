package mining

import (
	"regexp"
	"strings"
)

// 价格信号: ¥9.9 / ￥29 / 29元 / 9.9r / 💰
var priceRegex = regexp.MustCompile(`[¥￥]\s*\d+(\.\d+)?|\d+(\.\d+)?\s*元|\d+(\.\d+)?\s*[rR]|💰`)

// 联系方式信号词
var contactWords = []string{
	"私我", "私聊", "私信", "dd", "扣1", "主页", "咨询", "客服",
	"评论区", "vx", "微信", "+v",
}

// 商品类型信号词
var productWords = []string{
	"教程", "合集", "资料", "模板", "定制", "源码",
	"指令", "prompt", "提示词", "课程", "工具", "插件",
}

// ExtractSignals 从一段文本中抽取交易信号
func ExtractSignals(text string) TradeSignals {
	signals := TradeSignals{
		PriceHits:   priceRegex.FindAllString(text, -1),
		ContactHits: matchWords(text, contactWords),
		ProductHits: matchWords(text, productWords),
	}
	signals.SignalScore = signalScore(signals)
	return signals
}

func matchWords(text string, words []string) []string {
	lower := strings.ToLower(text)
	var hits []string
	for _, w := range words {
		if strings.Contains(lower, strings.ToLower(w)) {
			hits = append(hits, w)
		}
	}
	return hits
}

func signalScore(s TradeSignals) int {
	score := 0
	if len(s.PriceHits) > 0 {
		score++
	}
	if len(s.ContactHits) > 0 {
		score++
	}
	if len(s.ProductHits) > 0 {
		score++
	}
	return score
}

// PassPreFilter 笔记级预过滤
// 输入: 搜索结果返回的 title + 可选的 content。
// 阈值: 至少命中 1 类信号才进入详情抓取(节省 token 和请求)。
func PassPreFilter(title, content string) bool {
	return ExtractSignals(title+" "+content).SignalScore >= 1
}
