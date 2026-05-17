package analyzer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config DeepSeek API 配置(从 configs/deepseek.yml 读)
type Config struct {
	APIKey         string  `yaml:"api_key"`
	Model          string  `yaml:"model"`
	Endpoint       string  `yaml:"endpoint"`
	Temperature    float64 `yaml:"temperature"`
	TimeoutSeconds int     `yaml:"timeout_seconds"`
}

// LoadConfig 从 yaml 文件读取配置
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if c.APIKey == "" {
		return nil, fmt.Errorf("api_key 为空")
	}
	if c.Endpoint == "" {
		c.Endpoint = "https://api.deepseek.com/chat/completions"
	}
	if c.Model == "" {
		c.Model = "deepseek-chat"
	}
	if c.TimeoutSeconds == 0 {
		c.TimeoutSeconds = 60
	}
	return &c, nil
}

// Client 极简 DeepSeek API 客户端(OpenAI 兼容协议)
type Client struct {
	cfg  *Config
	http *http.Client
}

// NewClient 构造客户端
func NewClient(cfg *Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second},
	}
}

// ChatRequest OpenAI 兼容 chat 请求体
type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	Temperature    float64         `json:"temperature"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type string `json:"type"` // "json_object"
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
		FinishR string      `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// ChatJSON 发起 chat 请求,强制 JSON 输出
func (c *Client) ChatJSON(ctx context.Context, system, user string) (string, int, error) {
	body, err := json.Marshal(chatRequest{
		Model:       c.cfg.Model,
		Temperature: c.cfg.Temperature,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		ResponseFormat: &responseFormat{Type: "json_object"},
	})
	if err != nil {
		return "", 0, fmt.Errorf("marshal req: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("read resp: %w", err)
	}

	if resp.StatusCode >= 400 {
		return "", 0, fmt.Errorf("api status %d: %s", resp.StatusCode, string(raw))
	}

	var cr chatResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return "", 0, fmt.Errorf("unmarshal resp: %w, raw=%s", err, string(raw))
	}
	if cr.Error != nil {
		return "", 0, fmt.Errorf("api error: %s (%s)", cr.Error.Message, cr.Error.Type)
	}
	if len(cr.Choices) == 0 {
		return "", 0, fmt.Errorf("no choices in response: %s", string(raw))
	}
	return cr.Choices[0].Message.Content, cr.Usage.TotalTokens, nil
}
