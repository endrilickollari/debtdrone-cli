package analysis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	CacheVersion   = "v1"
	CacheTTL       = 7 * 24 * time.Hour // 7 days
	CacheKeyPrefix = "debtdrone:analysis"
)

type CachedResult struct {
	ComplexityScore float64   `json:"complexity_score"`
	LineCount       int       `json:"line_count"`
	CognitiveScore  float64   `json:"cognitive_score,omitempty"`
	Functions       int       `json:"functions,omitempty"`
	IssueCount      int       `json:"issue_count,omitempty"`
	CachedAt        time.Time `json:"cached_at"`
}

type AnalysisCache struct {
	redis   *redis.Client
	enabled bool
}

func NewAnalysisCache(redisClient *redis.Client) *AnalysisCache {
	return &AnalysisCache{
		redis:   redisClient,
		enabled: redisClient != nil,
	}
}

func HashFileContent(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

func (c *AnalysisCache) cacheKey(analyzerName, fileHash string) string {
	return fmt.Sprintf("%s:%s:%s:%s", CacheKeyPrefix, CacheVersion, analyzerName, fileHash)
}

func (c *AnalysisCache) Get(ctx context.Context, analyzerName, fileHash string) (*CachedResult, bool) {
	if !c.enabled {
		return nil, false
	}

	key := c.cacheKey(analyzerName, fileHash)
	data, err := c.redis.Get(ctx, key).Bytes()
	if err != nil {
		if err != redis.Nil {
			log.Printf("⚠️ [Cache] Redis get error: %v", err)
		}
		log.Printf("📊 [Cache Miss] %s hash=%s", analyzerName, fileHash[:12])
		return nil, false
	}

	var result CachedResult
	if err := json.Unmarshal(data, &result); err != nil {
		log.Printf("⚠️ [Cache] Unmarshal error: %v", err)
		return nil, false
	}

	log.Printf("✅ [Cache Hit] %s hash=%s complexity=%.1f lines=%d",
		analyzerName, fileHash[:12], result.ComplexityScore, result.LineCount)
	return &result, true
}

func (c *AnalysisCache) Set(ctx context.Context, analyzerName, fileHash string, result *CachedResult) error {
	if !c.enabled {
		return nil
	}

	result.CachedAt = time.Now()
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal cache result: %w", err)
	}

	key := c.cacheKey(analyzerName, fileHash)

	// Use SetNX for atomic operation to avoid race conditions
	set, err := c.redis.SetNX(ctx, key, data, CacheTTL).Result()
	if err != nil {
		return fmt.Errorf("redis setnx: %w", err)
	}

	if !set {
		// Key already exists, update with SET instead
		if err := c.redis.Set(ctx, key, data, CacheTTL).Err(); err != nil {
			return fmt.Errorf("redis set: %w", err)
		}
	}

	return nil
}

func (c *AnalysisCache) GetStats(ctx context.Context) (hits, misses int64, err error) {
	if !c.enabled {
		return 0, 0, nil
	}

	// Get cache stats from Redis INFO command
	info, err := c.redis.Info(ctx, "stats").Result()
	if err != nil {
		return 0, 0, err
	}

	// Parse keyspace_hits and keyspace_misses from info string
	_ = info // Stats parsing could be added here if needed
	return 0, 0, nil
}

func (c *AnalysisCache) InvalidateForRepo(ctx context.Context, repoID string) error {
	if !c.enabled {
		return nil
	}

	pattern := fmt.Sprintf("%s:*:%s:*", CacheKeyPrefix, repoID)
	iter := c.redis.Scan(ctx, 0, pattern, 100).Iterator()

	var keysToDelete []string
	for iter.Next(ctx) {
		keysToDelete = append(keysToDelete, iter.Val())
	}

	if err := iter.Err(); err != nil {
		return fmt.Errorf("scan keys: %w", err)
	}

	if len(keysToDelete) > 0 {
		if err := c.redis.Del(ctx, keysToDelete...).Err(); err != nil {
			return fmt.Errorf("delete keys: %w", err)
		}
		log.Printf("🗑️ [Cache] Invalidated %d cache entries for repo %s", len(keysToDelete), repoID)
	}

	return nil
}
