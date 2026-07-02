package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const captionCacheVersion = 1

type captionCacheState struct {
	Version   int               `json:"version"`
	Captions  map[string]string `json:"captions"`
	UpdatedAt string            `json:"updated_at,omitempty"`
}

type CaptionCache struct {
	path  string
	state captionCacheState
	mu    sync.Mutex
}

func OpenCaptionCache(path string) (*CaptionCache, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, fmt.Errorf("caption cache path is required")
	}
	cache := &CaptionCache{
		path: trimmed,
		state: captionCacheState{
			Version:  captionCacheVersion,
			Captions: map[string]string{},
		},
	}
	data, err := os.ReadFile(trimmed)
	if err != nil {
		if os.IsNotExist(err) {
			return cache, nil
		}
		return nil, fmt.Errorf("read caption cache: %w", err)
	}
	var state captionCacheState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse caption cache %s: %w", trimmed, err)
	}
	if state.Version != captionCacheVersion {
		return nil, fmt.Errorf("caption cache %s has unsupported version %d (want %d)", trimmed, state.Version, captionCacheVersion)
	}
	if state.Captions == nil {
		state.Captions = map[string]string{}
	}
	cache.state = state
	return cache, nil
}

func (c *CaptionCache) Count() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.state.Captions)
}

func (c *CaptionCache) Get(hash string) (string, bool) {
	if c == nil {
		return "", false
	}
	key := strings.TrimSpace(hash)
	if key == "" {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	value, ok := c.state.Captions[key]
	return value, ok
}

func (c *CaptionCache) Set(hash, caption string) error {
	if c == nil {
		return nil
	}
	key := strings.TrimSpace(hash)
	if key == "" {
		return fmt.Errorf("caption cache hash is required")
	}
	trimmedCaption := strings.TrimSpace(caption)
	if trimmedCaption == "" {
		return fmt.Errorf("caption cache value is empty")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.state.Captions[key]; ok && existing == trimmedCaption {
		return nil
	}
	c.state.Captions[key] = trimmedCaption
	c.state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return c.persistLocked()
}

func (c *CaptionCache) persistLocked() error {
	state := captionCacheState{
		Version:   captionCacheVersion,
		Captions:  make(map[string]string, len(c.state.Captions)),
		UpdatedAt: c.state.UpdatedAt,
	}
	keys := make([]string, 0, len(c.state.Captions))
	for key := range c.state.Captions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		state.Captions[key] = c.state.Captions[key]
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode caption cache: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return fmt.Errorf("prepare caption cache directory: %w", err)
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write caption cache: %w", err)
	}
	if err := os.Rename(tmp, c.path); err != nil {
		return fmt.Errorf("commit caption cache: %w", err)
	}
	return nil
}

type CaptionCacheStats struct {
	Hits   int
	Misses int
	Writes int
}

type CachingCaptioner struct {
	next  Captioner
	cache *CaptionCache
	mu    sync.Mutex
	stats CaptionCacheStats
}

func NewCachingCaptioner(next Captioner, cache *CaptionCache) *CachingCaptioner {
	return &CachingCaptioner{next: next, cache: cache}
}

func (c *CachingCaptioner) CaptionImage(ctx context.Context, attachment ResolvedAttachment) (string, error) {
	if c == nil || c.next == nil {
		return "", fmt.Errorf("captioner is required")
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(attachment.MimeType)), "image/") {
		return c.next.CaptionImage(ctx, attachment)
	}
	imageBytes, err := os.ReadFile(attachment.DiskPath)
	if err != nil {
		return "", fmt.Errorf("read attachment bytes: %w", err)
	}
	sum := sha256.Sum256(imageBytes)
	hash := hex.EncodeToString(sum[:])
	if cached, ok := c.cache.Get(hash); ok {
		c.recordHit()
		return cached, nil
	}
	c.recordMiss()
	caption, err := c.next.CaptionImage(ctx, attachment)
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(caption)
	if trimmed == "" {
		return "", nil
	}
	if err := c.cache.Set(hash, trimmed); err != nil {
		return "", err
	}
	c.recordWrite()
	return trimmed, nil
}

func (c *CachingCaptioner) CaptionCacheStats() CaptionCacheStats {
	if c == nil {
		return CaptionCacheStats{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stats
}

func (c *CachingCaptioner) recordHit() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stats.Hits++
}

func (c *CachingCaptioner) recordMiss() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stats.Misses++
}

func (c *CachingCaptioner) recordWrite() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stats.Writes++
}
