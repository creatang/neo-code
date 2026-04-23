package feishu

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DedupeStore 基于 TTL 的幂等去重存储。
type DedupeStore struct {
	ttl time.Duration

	mu      sync.Mutex
	entries map[string]time.Time
	path    string
	logger  *log.Logger
}

// NewDedupeStore 创建去重存储。
func NewDedupeStore(ttl time.Duration) *DedupeStore {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	return &DedupeStore{
		ttl:     ttl,
		entries: map[string]time.Time{},
	}
}

// NewPersistentDedupeStore 创建持久化去重存储。
func NewPersistentDedupeStore(ttl time.Duration, path string, logger *log.Logger) *DedupeStore {
	store := NewDedupeStore(ttl)
	store.path = strings.TrimSpace(path)
	store.logger = logger
	store.load()
	return store
}

// SeenOrAdd 返回 key 是否已存在；不存在时会写入。
func (d *DedupeStore) SeenOrAdd(key string, now time.Time) bool {
	if d.Seen(key, now) {
		return true
	}
	d.Add(key, now)
	return false
}

// Seen checks whether key is still within dedupe TTL window.
func (d *DedupeStore) Seen(key string, now time.Time) bool {
	if d == nil {
		return false
	}
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	d.cleanupLocked(now)
	if expiresAt, exists := d.entries[trimmed]; exists && now.Before(expiresAt) {
		return true
	}
	return false
}

// Add inserts key into dedupe window.
func (d *DedupeStore) Add(key string, now time.Time) {
	if d == nil {
		return
	}
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	d.cleanupLocked(now)
	d.entries[trimmed] = now.Add(d.ttl)
	d.persistLocked(now)
}

func (d *DedupeStore) cleanupLocked(now time.Time) {
	for key, expiresAt := range d.entries {
		if !now.Before(expiresAt) {
			delete(d.entries, key)
		}
	}
}

func (d *DedupeStore) load() {
	if d == nil || strings.TrimSpace(d.path) == "" {
		return
	}
	raw, err := os.ReadFile(d.path)
	if err != nil {
		if !os.IsNotExist(err) {
			d.logf("load dedupe store failed: %v", err)
		}
		return
	}
	var payload map[string]string
	if err := json.Unmarshal(raw, &payload); err != nil {
		d.logf("decode dedupe store failed: %v", err)
		return
	}
	now := time.Now().UTC()
	d.mu.Lock()
	defer d.mu.Unlock()
	for key, ts := range payload {
		expireAt, err := time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			continue
		}
		if now.Before(expireAt) {
			d.entries[strings.TrimSpace(key)] = expireAt
		}
	}
}

func (d *DedupeStore) persistLocked(now time.Time) {
	if d == nil || strings.TrimSpace(d.path) == "" {
		return
	}
	d.cleanupLocked(now)
	payload := map[string]string{}
	for key, expireAt := range d.entries {
		payload[key] = expireAt.UTC().Format(time.RFC3339Nano)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		d.logf("encode dedupe store failed: %v", err)
		return
	}
	path := strings.TrimSpace(d.path)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		d.logf("create dedupe store dir failed: %v", err)
		return
	}
	tempFile, err := os.CreateTemp(dir, ".feishu-dedupe-*.tmp")
	if err != nil {
		d.logf("create dedupe temp file failed: %v", err)
		return
	}
	tempPath := tempFile.Name()
	if _, err := tempFile.Write(raw); err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
		d.logf("write dedupe temp file failed: %v", err)
		return
	}
	if _, err := tempFile.Write([]byte("\n")); err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
		d.logf("write dedupe temp newline failed: %v", err)
		return
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		d.logf("close dedupe temp file failed: %v", err)
		return
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		d.logf("persist dedupe store failed: %v", err)
		return
	}
}

func (d *DedupeStore) logf(format string, args ...any) {
	if d == nil {
		return
	}
	if d.logger != nil {
		d.logger.Printf(format, args...)
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, "feishu-dedupe: "+format+"\n", args...)
}
