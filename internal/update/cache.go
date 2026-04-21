package update

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Cache stores the most recent update check result.
type Cache struct {
	CheckedAt       time.Time `json:"checkedAt"`
	NotifiedAt      time.Time `json:"notifiedAt,omitempty"`
	CurrentVersion  string    `json:"currentVersion"`
	LatestVersion   string    `json:"latestVersion"`
	UpdateAvailable bool      `json:"updateAvailable"`
	Comparable      bool      `json:"comparable"`
	ReleaseURL      string    `json:"releaseUrl"`
	AssetName       string    `json:"assetName,omitempty"`
}

// CheckWithCache returns a cached update check when it is still fresh.
func CheckWithCache(ctx context.Context, client *Client, currentVersion, cachePath string, ttl time.Duration) (*CheckResult, error) {
	if cached, ok := readFreshCache(cachePath, currentVersion, ttl); ok {
		return cached, nil
	}

	result, err := client.Check(ctx, currentVersion)
	if err != nil {
		return nil, err
	}
	_ = writeCache(cachePath, result)
	return result, nil
}

// NotificationDue reports whether result should be shown to the user.
func NotificationDue(cachePath string, result *CheckResult, interval time.Duration) bool {
	if result == nil || !result.UpdateAvailable {
		return false
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		return true
	}
	var cache Cache
	if err := json.Unmarshal(data, &cache); err != nil {
		return true
	}
	if cache.LatestVersion != result.LatestVersion {
		return true
	}
	return cache.NotifiedAt.IsZero() || time.Since(cache.NotifiedAt) > interval
}

// MarkNotified records that the user has seen an update notice.
func MarkNotified(cachePath string, result *CheckResult) error {
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return writeCacheWithNotice(cachePath, result, time.Now(), time.Now())
	}
	var cache Cache
	if err := json.Unmarshal(data, &cache); err != nil {
		return writeCacheWithNotice(cachePath, result, time.Now(), time.Now())
	}
	cache.NotifiedAt = time.Now()
	return writeCacheObject(cachePath, &cache)
}

func readFreshCache(cachePath, currentVersion string, ttl time.Duration) (*CheckResult, bool) {
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, false
	}
	var cache Cache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, false
	}
	if cache.CurrentVersion != currentVersion || time.Since(cache.CheckedAt) > ttl {
		return nil, false
	}
	return &CheckResult{
		CurrentVersion:  cache.CurrentVersion,
		LatestVersion:   cache.LatestVersion,
		UpdateAvailable: cache.UpdateAvailable,
		Comparable:      cache.Comparable,
		ReleaseURL:      cache.ReleaseURL,
		AssetName:       cache.AssetName,
	}, true
}

func writeCache(cachePath string, result *CheckResult) error {
	return writeCacheWithNotice(cachePath, result, time.Now(), time.Time{})
}

func writeCacheWithNotice(cachePath string, result *CheckResult, checkedAt, notifiedAt time.Time) error {
	return writeCacheObject(cachePath, &Cache{
		CheckedAt:       checkedAt,
		NotifiedAt:      notifiedAt,
		CurrentVersion:  result.CurrentVersion,
		LatestVersion:   result.LatestVersion,
		UpdateAvailable: result.UpdateAvailable,
		Comparable:      result.Comparable,
		ReleaseURL:      result.ReleaseURL,
		AssetName:       result.AssetName,
	})
}

func writeCacheObject(cachePath string, cache *Cache) error {
	if err := os.MkdirAll(filepath.Dir(cachePath), 0700); err != nil {
		return fmt.Errorf("failed to create update cache directory: %w", err)
	}
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal update cache: %w", err)
	}
	if err := os.WriteFile(cachePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write update cache: %w", err)
	}
	return nil
}
