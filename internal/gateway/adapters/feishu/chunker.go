package feishu

import (
	"strings"
	"sync"
	"time"
)

// ChunkAggregator 用于对流式 chunk 做聚合与节流。
type ChunkAggregator struct {
	maxChars int
	interval time.Duration

	mu        sync.Mutex
	buffer    strings.Builder
	lastFlush time.Time
}

// NewChunkAggregator 创建聚合器。
func NewChunkAggregator(maxChars int, interval time.Duration) *ChunkAggregator {
	if maxChars <= 0 {
		maxChars = defaultChunkFlushMaxChars
	}
	if interval <= 0 {
		interval = defaultChunkFlushInterval
	}
	return &ChunkAggregator{
		maxChars: maxChars,
		interval: interval,
	}
}

// Add 写入 chunk，并在满足阈值时返回可刷新的文本。
func (c *ChunkAggregator) Add(chunk string, now time.Time) (string, bool) {
	if c == nil {
		return "", false
	}
	trimmed := strings.TrimSpace(chunk)
	if trimmed == "" {
		return "", false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.buffer.Len() > 0 {
		c.buffer.WriteByte('\n')
	}
	c.buffer.WriteString(trimmed)

	if c.shouldFlushLocked(now) {
		return c.flushLocked(now), true
	}
	return "", false
}

// Flush 主动刷出缓存。
func (c *ChunkAggregator) Flush(now time.Time) string {
	if c == nil {
		return ""
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.flushLocked(now)
}

func (c *ChunkAggregator) shouldFlushLocked(now time.Time) bool {
	if c.buffer.Len() >= c.maxChars {
		return true
	}
	if c.lastFlush.IsZero() {
		return false
	}
	return now.Sub(c.lastFlush) >= c.interval
}

func (c *ChunkAggregator) flushLocked(now time.Time) string {
	content := strings.TrimSpace(c.buffer.String())
	c.buffer.Reset()
	c.lastFlush = now.UTC()
	return content
}
