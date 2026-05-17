// cmd/web: 极简 dashboard,展示 LLM 已分析的机会列表(按综合分数倒序)
// 用法:
//
//	go run ./cmd/web -db data/mining.db -addr :8088
package main

import (
	"embed"
	"encoding/json"
	"flag"
	"html/template"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/xpzouying/xiaohongshu-mcp/mining"
	"github.com/xpzouying/xiaohongshu-mcp/mining/analyzer"
)

//go:embed templates/*.html
var tmplFS embed.FS

type viewItem struct {
	Note           *mining.Note
	Op             *analyzer.Opportunity
	CompositeScore int
}

func main() {
	var (
		dbPath = flag.String("db", "data/mining.db", "SQLite 数据库路径")
		addr   = flag.String("addr", ":8088", "监听地址")
		limit  = flag.Int("limit", 100, "首页最多展示条数")
	)
	flag.Parse()

	store, err := mining.OpenStore(*dbPath)
	if err != nil {
		logrus.Fatalf("打开数据库失败: %v", err)
	}
	defer store.Close()

	tmpl := template.Must(template.ParseFS(tmplFS, "templates/*.html"))

	r := gin.Default()
	r.SetHTMLTemplate(tmpl)

	r.GET("/", func(c *gin.Context) {
		records, err := store.ListOpportunities(*limit)
		if err != nil {
			c.String(http.StatusInternalServerError, "查询失败: %v", err)
			return
		}
		items := make([]viewItem, 0, len(records))
		for _, rec := range records {
			var op analyzer.Opportunity
			if err := json.Unmarshal([]byte(rec.OpportunityRaw), &op); err != nil {
				logrus.Warnf("解析 opportunity 失败 %s: %v", rec.Note.NoteID, err)
				continue
			}
			items = append(items, viewItem{
				Note:           rec.Note,
				Op:             &op,
				CompositeScore: rec.CompositeScore,
			})
		}
		c.HTML(http.StatusOK, "index.html", gin.H{
			"Items":  items,
			"DBPath": *dbPath,
		})
	})

	logrus.Infof("dashboard 启动: http://localhost%s", *addr)
	if err := r.Run(*addr); err != nil {
		logrus.Fatal(err)
	}
}
