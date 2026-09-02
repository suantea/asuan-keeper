// Package aicache 提供基于 AI 的智能预缓存策略。
//
// 核心思路：
//  1. 记录文件访问日志（哪些文件被访问、何时、频率）
//  2. 定期把访问模式发送给 DeepSeek，请求预测"接下来可能需要的文件"
//  3. 预测结果用于预水合（hydrate）——提前把占位符恢复为本地文件
//
// 与 autorelease 互补：autorelease 释放冷文件，aicache 预热热文件。
package aicache

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// AccessLog 记录单次文件访问。
type AccessLog struct {
	Path      string    `json:"path"`       // 相对路径（/ 分隔）
	FolderID  string    `json:"folder_id"`
	Timestamp time.Time `json:"timestamp"`
}

// AccessStats 聚合后的文件访问统计。
type AccessStats struct {
	Path       string  `json:"path"`
	FolderID   string  `json:"folder_id"`
	Count      int     `json:"count"`
	LastAccess time.Time `json:"last_access"`
	HourlyDist [24]int `json:"hourly_dist"` // 24 小时分布
}

// Prediction AI 预测结果。
type Prediction struct {
	Path     string  `json:"path"`
	FolderID string  `json:"folder_id"`
	Score    float64 `json:"score"`    // 0-1，预测被访问概率
	Reason   string  `json:"reason"`
}

// PreCacheResult 预缓存执行结果。
type PreCacheResult struct {
	Path    string `json:"path"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// Config AI 预缓存配置。
type Config struct {
	Enabled       bool   `json:"enabled"`
	APIKey        string `json:"api_key"`
	BaseURL       string `json:"base_url"`       // 默认 https://api.deepseek.com
	Model         string `json:"model"`           // 默认 deepseek-chat
	AnalyzeEvery  string `json:"analyze_every"`   // 分析间隔，如 "1h"
	TopN          int    `json:"top_n"`           // 每次预测取前 N 个文件
	MaxHistory    int    `json:"max_history"`     // 最大历史记录数
}

func DefaultConfig() Config {
	return Config{
		BaseURL:      "https://api.deepseek.com",
		Model:        "deepseek-chat",
		AnalyzeEvery: "1h",
		TopN:         10,
		MaxHistory:   1000,
	}
}

// Tracker 文件访问追踪器。
type Tracker struct {
	mu      sync.Mutex
	logs    []AccessLog
	config  Config
	client  *http.Client
	hydrate func(folderID, relPath string) error // 预水合回调
}

// NewTracker 创建追踪器。hydrate 回调负责实际的文件预缓存。
func NewTracker(cfg Config, hydrate func(folderID, relPath string) error) *Tracker {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.deepseek.com"
	}
	if cfg.Model == "" {
		cfg.Model = "deepseek-chat"
	}
	if cfg.MaxHistory <= 0 {
		cfg.MaxHistory = 1000
	}
	return &Tracker{
		logs:    make([]AccessLog, 0, 64),
		config:  cfg,
		client:  &http.Client{Timeout: 30 * time.Second},
		hydrate: hydrate,
	}
}

// Record 记录一次文件访问。
func (t *Tracker) Record(folderID, relPath string) {
	if !t.config.Enabled {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.logs = append(t.logs, AccessLog{
		Path:      relPath,
		FolderID:  folderID,
		Timestamp: time.Now(),
	})
	// 淘汰旧记录
	if len(t.logs) > t.config.MaxHistory {
		t.logs = t.logs[len(t.logs)-t.config.MaxHistory:]
	}
}

// AnalyzeAndPreCache 执行一次分析+预缓存周期。
func (t *Tracker) AnalyzeAndPreCache() ([]PreCacheResult, error) {
	if !t.config.Enabled || t.config.APIKey == "" {
		return nil, nil
	}

	// 1. 聚合访问统计
	stats := t.aggregateStats()
	if len(stats) == 0 {
		return nil, nil
	}

	// 2. 调用 AI 预测
	predictions, err := t.predict(stats)
	if err != nil {
		return nil, fmt.Errorf("AI 预测失败: %w", err)
	}
	if len(predictions) == 0 {
		return nil, nil
	}

	// 3. 执行预缓存
	var results []PreCacheResult
	for _, p := range predictions {
		r := PreCacheResult{Path: p.Path}
		if t.hydrate != nil {
			if err := t.hydrate(p.FolderID, p.Path); err != nil {
				r.Error = err.Error()
			} else {
				r.Success = true
			}
		}
		results = append(results, r)
	}
	return results, nil
}

// aggregateStats 聚合访问日志为统计信息。
func (t *Tracker) aggregateStats() []AccessStats {
	t.mu.Lock()
	logs := make([]AccessLog, len(t.logs))
	copy(logs, t.logs)
	t.mu.Unlock()

	if len(logs) == 0 {
		return nil
	}

	type key struct {
		path     string
		folderID string
	}
	statsMap := make(map[key]*AccessStats)
	for _, l := range logs {
		k := key{l.Path, l.FolderID}
		s, ok := statsMap[k]
		if !ok {
			s = &AccessStats{Path: l.Path, FolderID: l.FolderID}
			statsMap[k] = s
		}
		s.Count++
		s.HourlyDist[l.Timestamp.Hour()]++
		if l.Timestamp.After(s.LastAccess) {
			s.LastAccess = l.Timestamp
		}
	}

	var stats []AccessStats
	for _, s := range statsMap {
		stats = append(stats, *s)
	}
	// 按访问次数降序
	sort.Slice(stats, func(i, j int) bool { return stats[i].Count > stats[j].Count })
	if len(stats) > 50 {
		stats = stats[:50]
	}
	return stats
}

// predict 调用 DeepSeek 预测下一步可能访问的文件。
func (t *Tracker) predict(stats []AccessStats) ([]Prediction, error) {
	statsJSON, _ := json.MarshalIndent(stats, "", "  ")

	prompt := fmt.Sprintf(`分析以下文件访问记录，预测接下来最可能被访问的文件。

访问统计（按频率降序）：
%s

要求：
1. 只返回 JSON 数组，不要其他内容
2. 每个元素包含 path、folder_id、score（0-1 概率）、reason（简短中文原因）
3. 按 score 降序，最多返回 %d 个
4. 考虑时间模式（当前小时 %d 点）、访问频率、最近性
5. 优先预测尚未被频繁访问但符合使用模式的文件

输出格式：[{"path":"...","folder_id":"...","score":0.8,"reason":"..."}]`,
		string(statsJSON), t.config.TopN, time.Now().Hour())

	body, _ := json.Marshal(map[string]any{
		"model": t.config.Model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	})

	req, err := http.NewRequest("POST", t.config.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.config.APIKey)

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API %d: %s", resp.StatusCode, string(b))
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, err
	}
	if len(chatResp.Choices) == 0 {
		return nil, nil
	}

	// 解析 AI 返回的 JSON 数组
	content := chatResp.Choices[0].Message.Content
	// 提取 JSON 部分（AI 可能包裹在 ```json ... ``` 中）
	if idx := strings.Index(content, "["); idx >= 0 {
		if end := strings.LastIndex(content, "]"); end > idx {
			content = content[idx : end+1]
		}
	}

	var predictions []Prediction
	if err := json.Unmarshal([]byte(content), &predictions); err != nil {
		return nil, fmt.Errorf("解析 AI 预测结果失败: %w (内容: %s)", err, content[:min(200, len(content))])
	}

	// 限制数量
	if len(predictions) > t.config.TopN {
		predictions = predictions[:t.config.TopN]
	}
	return predictions, nil
}

// Stats 返回当前追踪器的统计摘要。
func (t *Tracker) Stats() map[string]any {
	t.mu.Lock()
	defer t.mu.Unlock()
	return map[string]any{
		"enabled":     t.config.Enabled,
		"log_count":   len(t.logs),
		"max_history": t.config.MaxHistory,
		"ai_configured": t.config.APIKey != "",
	}
}

// SaveToFile 持久化访问日志到文件（进程重启不丢失）。
func (t *Tracker) SaveToFile(path string) error {
	t.mu.Lock()
	logs := make([]AccessLog, len(t.logs))
	copy(logs, t.logs)
	t.mu.Unlock()

	b, err := json.MarshalIndent(logs, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// LoadFromFile 从文件恢复访问日志。
func (t *Tracker) LoadFromFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var logs []AccessLog
	if err := json.Unmarshal(b, &logs); err != nil {
		return err
	}
	t.mu.Lock()
	t.logs = logs
	t.mu.Unlock()
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
