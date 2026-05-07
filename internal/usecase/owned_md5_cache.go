package usecase

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/port"
)

// OwnedCacheStatus は GUI 表示用のキャッシュ状態スナップショット。
type OwnedCacheStatus struct {
	Loaded     bool
	Count      int
	LoadedAt   *time.Time
	LoadedPath string
	LastError  string
}

// OwnedMD5Cache は port.OwnedChartRepo の上に薄いキャッシュを被せた usecase。
// auto-load + 明示的な Reload + Invalidate（設定変更 hook）+ Status を提供する。
type OwnedMD5Cache struct {
	repo  port.OwnedChartRepo
	cfg   port.ConfigStore
	clock port.Clock
	log   *slog.Logger

	mu         sync.RWMutex
	loaded     bool
	set        map[string]struct{}
	loadedAt   *time.Time
	loadedPath string
	lastErr    string
}

// NewOwnedMD5Cache は新しい OwnedMD5Cache を作る。
func NewOwnedMD5Cache(
	repo port.OwnedChartRepo,
	cfg port.ConfigStore,
	clock port.Clock,
	log *slog.Logger,
) *OwnedMD5Cache {
	return &OwnedMD5Cache{repo: repo, cfg: cfg, clock: clock, log: log}
}

// Get は md5 集合を返す。未ロードなら 1 度だけ自動でロードする。
// Reload 失敗時は前回の set を保持しつつ lastErr のみ更新するため、Get は基本的に成功する。
func (c *OwnedMD5Cache) Get(ctx context.Context) (map[string]struct{}, error) {
	c.mu.RLock()
	if c.loaded {
		out := copySet(c.set)
		c.mu.RUnlock()
		return out, nil
	}
	c.mu.RUnlock()

	if err := c.Reload(ctx); err != nil {
		// 一度もロードできていない場合だけエラー伝播。
		// それ以前のロード成功 set があれば保持して正常応答する。
		c.mu.RLock()
		hasSet := c.set != nil
		c.mu.RUnlock()
		if !hasSet {
			return nil, err
		}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return copySet(c.set), nil
}

// Reload は ConfigStore から最新パスを取得して repo を呼ぶ。
// 失敗時は前回の set を保持し、lastErr のみ更新する。
func (c *OwnedMD5Cache) Reload(ctx context.Context) error {
	dbPath, _, err := c.cfg.Get(ctx, "songdata_db_path")
	if err != nil {
		c.recordError(err.Error())
		return err
	}
	got, err := c.repo.LoadOwnedMD5Set(ctx, dbPath)
	if err != nil {
		c.recordError(err.Error())
		c.log.Warn("owned md5 reload failed", "err", err, "path", dbPath)
		return err
	}
	now := c.clock.Now()
	c.mu.Lock()
	c.set = got
	c.loaded = true
	c.loadedAt = &now
	c.loadedPath = dbPath
	c.lastErr = ""
	c.mu.Unlock()
	c.log.Info("owned md5 reloaded", "count", len(got), "path", dbPath)
	return nil
}

// Invalidate は set を未ロード状態に戻す（次回 Get / Reload で repo を呼び直す）。
// 設定の songdata_db_path が変更されたときに ConfigUseCase 経由で呼ばれる想定。
func (c *OwnedMD5Cache) Invalidate() {
	c.mu.Lock()
	c.loaded = false
	c.set = nil
	c.loadedAt = nil
	c.loadedPath = ""
	c.lastErr = ""
	c.mu.Unlock()
}

// Status は GUI 表示用のスナップショットを返す。
func (c *OwnedMD5Cache) Status() OwnedCacheStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return OwnedCacheStatus{
		Loaded:     c.loaded,
		Count:      len(c.set),
		LoadedAt:   c.loadedAt,
		LoadedPath: c.loadedPath,
		LastError:  c.lastErr,
	}
}

func (c *OwnedMD5Cache) recordError(msg string) {
	c.mu.Lock()
	c.lastErr = msg
	c.mu.Unlock()
}

func copySet(in map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(in))
	for k := range in {
		out[k] = struct{}{}
	}
	return out
}
