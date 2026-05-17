package analyzer

// Opportunity LLM 分析输出 - 一条小红书笔记的"机会"判断
// 设计依据: Step 2 设计文档 schema
type Opportunity struct {
	// 商业意图判断
	IsCommercialIntent bool     `json:"is_commercial_intent"`       // 是否含商业意图(免费引流也算)
	MonetizationType   string   `json:"monetization_type"`          // 免费分享/私聊售卖/外链引流/评论区交易/群聊引流/账号涨粉
	WhatSells          string   `json:"what_sells"`                 // 一句话讲清楚卖什么
	DeliveryForm       string   `json:"delivery_form"`              // 评论区编号自取/私聊一对一/网盘链接/微信群/站外链接/无
	PriceObserved      string   `json:"price_observed,omitempty"`   // 观察到的价格,如 "¥9.9" / "免费"
	ContactChannels    []string `json:"contact_channels,omitempty"` // 联系方式: 评论区/私信/主页

	// 需求强度
	DemandEvidence string `json:"demand_evidence"` // 需求验证依据
	DemandStrength int    `json:"demand_strength"` // 1-10

	// 技术化优化空间
	TechOptimizationScore   int      `json:"tech_optimization_score"`   // 1-10
	TechOptimizationReasons []string `json:"tech_optimization_reasons"` // 为什么有/没有空间
	ProductIdeas            []string `json:"product_ideas"`             // 具体产品化想法
	SimilarExistingExamples []string `json:"similar_existing_examples"` // 参考案例

	// 综合判定
	Verdict       string `json:"verdict"`        // 高价值/中等/跳过
	VerdictReason string `json:"verdict_reason"` // 简短原因
}

// CompositeScore 综合分数(用于排序),取需求强度 × 0.4 + 技术化空间 × 0.6
// 这是个工程评分,不是 LLM 给的,在写库时计算。
func (o *Opportunity) CompositeScore() int {
	return o.DemandStrength*4 + o.TechOptimizationScore*6
}
