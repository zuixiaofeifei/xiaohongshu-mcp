package mining

import (
	"os"

	"gopkg.in/yaml.v3"
)

// KeywordsConfig 种子词配置(分类 × 交易信号 笛卡尔积生成搜索词)
type KeywordsConfig struct {
	Categories   []Category `yaml:"categories"`
	TradeSignals []string   `yaml:"trade_signals"`
}

// Category 单个分类下的场景词集合
type Category struct {
	Name       string   `yaml:"name"`
	SceneWords []string `yaml:"scene_words"`
}

// SearchQuery 一条具体的搜索任务
type SearchQuery struct {
	Category string
	Keyword  string
}

// LoadKeywords 从 YAML 文件加载种子词配置
func LoadKeywords(path string) (*KeywordsConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg KeywordsConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// BuildSearchQueries 生成搜索任务列表
// 规则: 每个 category 的 scene_words 与 trade_signals 做笛卡尔积。
// 若 trade_signals 为空,则只用 scene 词搜索。
func (c *KeywordsConfig) BuildSearchQueries() []SearchQuery {
	var queries []SearchQuery
	for _, cat := range c.Categories {
		for _, scene := range cat.SceneWords {
			if len(c.TradeSignals) == 0 {
				queries = append(queries, SearchQuery{Category: cat.Name, Keyword: scene})
				continue
			}
			for _, signal := range c.TradeSignals {
				queries = append(queries, SearchQuery{
					Category: cat.Name,
					Keyword:  scene + " " + signal,
				})
			}
		}
	}
	return queries
}
