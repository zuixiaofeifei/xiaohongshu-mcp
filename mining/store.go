package mining

import (
	"database/sql"
	"encoding/json"
	"time"

	_ "modernc.org/sqlite"
)

// Store SQLite 存储层
type Store struct {
	db *sql.DB
}

// OpenStore 打开数据库并完成建表
func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close 关闭数据库
func (s *Store) Close() error { return s.db.Close() }

// migrate 建表(幂等)
// notes:    抓取后的标准化笔记(LLM 分析的输入)
// keywords: 种子词运行记录(用于调度)
// analysis: 后续 LLM 输出占位(本期未使用,提前建好)
func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS notes (
			note_id        TEXT PRIMARY KEY,
			url            TEXT,
			category_seed  TEXT,
			signal_score   INTEGER,
			raw_json       TEXT,
			fetched_at     TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS keywords (
			keyword        TEXT,
			category       TEXT,
			last_run_at    TIMESTAMP,
			enabled        INTEGER DEFAULT 1,
			PRIMARY KEY (keyword, category)
		)`,
		`CREATE TABLE IF NOT EXISTS analysis (
			note_id        TEXT PRIMARY KEY,
			llm_version    TEXT,
			opportunity    TEXT,
			score          INTEGER,
			created_at     TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_notes_signal_score ON notes(signal_score DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_notes_category ON notes(category_seed)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// SaveNote 写入/更新一条笔记
func (s *Store) SaveNote(n *Note) error {
	raw, err := json.Marshal(n)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO notes (note_id, url, category_seed, signal_score, raw_json, fetched_at)
		 VALUES (?,?,?,?,?,?)
		 ON CONFLICT(note_id) DO UPDATE SET
		   signal_score = excluded.signal_score,
		   raw_json     = excluded.raw_json,
		   fetched_at   = excluded.fetched_at`,
		n.NoteID, n.URL, n.CategorySeed, n.TradeSignals.SignalScore, string(raw), n.FetchedAt,
	)
	return err
}

// NoteFetchedWithin 判断笔记最近 within 时间内是否已抓过(用于增量去重)
func (s *Store) NoteFetchedWithin(noteID string, within time.Duration) (bool, error) {
	var t time.Time
	err := s.db.QueryRow(`SELECT fetched_at FROM notes WHERE note_id = ?`, noteID).Scan(&t)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return time.Since(t) < within, nil
}
