package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
)

// SongdataAttacher はメイン *sql.DB に対する songdata.db の ATTACH/DETACH ライフサイクルを管理する。
// SetMaxOpenConns(1) 前提 (ATTACH はコネクション単位)。
// 通常運用は RO ATTACH で行い、書込み権限は持たない (beatoraja 並行稼働中の DB 破壊事故防止)。
// 性能インデックス (md5, sha256) は Attach 時に一時的に独立した RW 接続を開いて
// CREATE INDEX IF NOT EXISTS を実行して即 close する。RW 期間は数ミリ秒に限定される。
// GUI 表示用に最終アタッチ状態とエラーをスナップショット保持する。
type SongdataAttacher struct {
	db    *sql.DB
	clock port.Clock
	log   *slog.Logger

	mu         sync.RWMutex
	attached   bool
	path       string
	songCount  int
	attachedAt *time.Time
	lastErr    string
}

// NewSongdataAttacher は新しい SongdataAttacher を作る。
func NewSongdataAttacher(db *sql.DB, clk port.Clock, log *slog.Logger) *SongdataAttacher {
	return &SongdataAttacher{db: db, clock: clk, log: log}
}

// Attach は songdata.db を schema 'sd' として RO ATTACH する。
// path が空なら何もしない (失敗ではない)。
// 既にアタッチされている状態で呼ばれた場合は一度 DETACH してから ATTACH し直す。
func (a *SongdataAttacher) Attach(ctx context.Context, path string) error {
	if path == "" {
		return nil
	}
	if a.IsAttached() {
		if err := a.Detach(ctx); err != nil {
			return err
		}
	}

	// 性能インデックスを独立 RW 接続で確保してから即 close。
	// beatoraja 並行稼働中でも通常の ATTACH は RO のまま維持できる。
	a.ensurePerformanceIndex(ctx, path)

	// 通常運用は RO。書込み権限を持たないことで beatoraja DB の破壊事故を物理的に防ぐ。
	dsn := fmt.Sprintf("file:%s?mode=ro", url.QueryEscape(path))
	if _, err := a.db.ExecContext(ctx, "ATTACH DATABASE ? AS sd", dsn); err != nil {
		a.recordError(err.Error())
		return fmt.Errorf("attach songdata %q: %w", path, err)
	}

	var count int
	row := a.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sd.song")
	if err := row.Scan(&count); err != nil {
		// COUNT 失敗時は ATTACH 状態を維持しつつエラー記録 (テーブル不在等)
		a.recordError(fmt.Sprintf("count sd.song: %v", err))
		count = 0
	}

	now := a.clock.Now()
	a.mu.Lock()
	a.attached = true
	a.path = path
	a.songCount = count
	a.attachedAt = &now
	a.lastErr = ""
	a.mu.Unlock()
	a.log.Info("songdata attached", "path", path, "count", count)
	return nil
}

// Detach は schema 'sd' を DETACH する。未アタッチなら no-op。
func (a *SongdataAttacher) Detach(ctx context.Context) error {
	if !a.IsAttached() {
		return nil
	}
	if _, err := a.db.ExecContext(ctx, "DETACH DATABASE sd"); err != nil {
		return fmt.Errorf("detach songdata: %w", err)
	}
	a.mu.Lock()
	a.attached = false
	a.path = ""
	a.songCount = 0
	a.attachedAt = nil
	a.mu.Unlock()
	a.log.Info("songdata detached")
	return nil
}

// ReAttach は Detach → Attach を 1 連の操作で行う (設定変更時のフック用)。
// path が空のときは Detach のみ行う。
func (a *SongdataAttacher) ReAttach(ctx context.Context, path string) error {
	if err := a.Detach(ctx); err != nil {
		return err
	}
	return a.Attach(ctx, path)
}

// IsAttached は現在 'sd' がアタッチされているかを返す。
func (a *SongdataAttacher) IsAttached() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.attached
}

// Status は GUI 表示用のスナップショットを返す。
func (a *SongdataAttacher) Status() domain.SongdataAttachStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return domain.SongdataAttachStatus{
		Attached:   a.attached,
		Path:       a.path,
		SongCount:  a.songCount,
		AttachedAt: a.attachedAt,
		LastError:  a.lastErr,
	}
}

func (a *SongdataAttacher) recordError(msg string) {
	a.mu.Lock()
	a.lastErr = msg
	a.mu.Unlock()
}

// ensurePerformanceIndex は songdata.db に対して LoadCharts EXISTS 高速化用の
// (md5, sha256) 複合インデックスを保証する。
// ATTACH 経由ではなく一時 *sql.DB を RW で短時間開いて CREATE INDEX IF NOT EXISTS を
// 実行して即 close する。これにより通常稼働中の attacher は RO のままで、
// beatoraja 並行稼働中のうっかり書込み事故を物理的に防ぐ。
// 失敗 (beatoraja の write lock 競合等) は warn のみで継続: インデックス未作成だと
// 単に EXISTS が full scan に戻るだけで動作はする。次回起動時に再試行される。
func (a *SongdataAttacher) ensurePerformanceIndex(ctx context.Context, path string) {
	dsn := fmt.Sprintf("file:%s?mode=rw", url.QueryEscape(path))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		a.log.Warn("ensure perf index: open rw failed", "err", err, "path", path)
		return
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx,
		"CREATE INDEX IF NOT EXISTS idx_song_md5_sha256 ON song(md5, sha256)",
	); err != nil {
		a.log.Warn("ensure perf index: create failed", "err", err, "path", path)
	}
}
