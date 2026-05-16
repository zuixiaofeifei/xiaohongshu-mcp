package mining

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// JSONDumpSink 把每条 Note 单独落盘为一个 JSON 文件,方便人工 review。
// 文件名: {note_id}.json
type JSONDumpSink struct {
	Dir string
}

// NewJSONDumpSink 准备好目录
func NewJSONDumpSink(dir string) (*JSONDumpSink, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &JSONDumpSink{Dir: dir}, nil
}

// SaveNote 实现 NoteSink
func (s *JSONDumpSink) SaveNote(n *Note) error {
	data, err := json.MarshalIndent(n, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.Dir, n.NoteID+".json"), data, 0o644)
}
