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

// NoteWithAnalysis 带 LLM 分析结果的笔记(供 web dashboard 使用)
type NoteWithAnalysis struct {
	Note           *Note
	OpportunityRaw string // LLM 输出的原始 JSON
	CompositeScore int
	LLMVersion     string
	AnalyzedAt     time.Time
}

// ListNotesNeedAnalysis 列出"应该被分析但还没分析"的笔记
// 条件: signal_score >= minSignal AND (没 analysis 记录 OR analysis.llm_version != 当前版本)
func (s *Store) ListNotesNeedAnalysis(minSignal int, llmVersion string, limit int) ([]*Note, error) {
	rows, err := s.db.Query(`
		SELECT n.raw_json
		FROM notes n
		LEFT JOIN analysis a ON a.note_id = n.note_id AND a.llm_version = ?
		WHERE n.signal_score >= ? AND a.note_id IS NULL
		ORDER BY n.signal_score DESC, n.fetched_at DESC
		LIMIT ?`, llmVersion, minSignal, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Note
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var n Note
		if err := json.Unmarshal([]byte(raw), &n); err != nil {
			return nil, err
		}
		out = append(out, &n)
	}
	return out, rows.Err()
}

// SaveAnalysis 写入或更新一条 LLM 分析结果
func (s *Store) SaveAnalysis(noteID, llmVersion string, opportunityJSON string, compositeScore int) error {
	_, err := s.db.Exec(`
		INSERT INTO analysis (note_id, llm_version, opportunity, score, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(note_id) DO UPDATE SET
		  llm_version = excluded.llm_version,
		  opportunity = excluded.opportunity,
		  score       = excluded.score,
		  created_at  = excluded.created_at`,
		noteID, llmVersion, opportunityJSON, compositeScore, time.Now(),
	)
	return err
}

// ListOpportunities 按综合分数倒序列出已分析的机会(供 web dashboard)
func (s *Store) ListOpportunities(limit int) ([]*NoteWithAnalysis, error) {
	rows, err := s.db.Query(`
		SELECT n.raw_json, a.opportunity, a.score, a.llm_version, a.created_at
		FROM analysis a
		JOIN notes n ON n.note_id = a.note_id
		ORDER BY a.score DESC, a.created_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*NoteWithAnalysis
	for rows.Next() {
		var (
			rawNote, oppJSON, llmVersion string
			score                        int
			createdAt                    time.Time
		)
		if err := rows.Scan(&rawNote, &oppJSON, &score, &llmVersion, &createdAt); err != nil {
			return nil, err
		}
		var n Note
		if err := json.Unmarshal([]byte(rawNote), &n); err != nil {
			return nil, err
		}
		out = append(out, &NoteWithAnalysis{
			Note:           &n,
			OpportunityRaw: oppJSON,
			CompositeScore: score,
			LLMVersion:     llmVersion,
			AnalyzedAt:     createdAt,
		})
	}
	return out, rows.Err()
}
