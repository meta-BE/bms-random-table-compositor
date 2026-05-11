package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/port"
)

// ScoreDBAttacher はメイン *sql.DB に対する score.db の ATTACH/DETACH ライフサイクルを管理する。
// schema 名は 'sc'。SongdataAttacher と同様に SetMaxOpenConns(1) 前提・RO 専用。
// beatoraja の score.db を破壊しないため RW では絶対に開かない。
type ScoreDBAttacher struct {
	db    *sql.DB
	clock port.Clock
	log   *slog.Logger

	mu         sync.RWMutex
	attached   bool
	path       string
	rowCount   int
	attachedAt *time.Time
	lastErr    string
}

// NewScoreDBAttacher は新しい ScoreDBAttacher を作る。
func NewScoreDBAttacher(db *sql.DB, clk port.Clock, log *slog.Logger) *ScoreDBAttacher {
	return &ScoreDBAttacher{db: db, clock: clk, log: log}
}

// Attach は score.db を schema 'sc' として RO ATTACH する。
// path が空なら no-op (失敗ではない)。
// 既にアタッチ済みなら一度 Detach してから再 ATTACH する。
func (a *ScoreDBAttacher) Attach(ctx context.Context, path string) error {
	if path == "" {
		return nil
	}
	if a.IsAttached() {
		if err := a.Detach(ctx); err != nil {
			return err
		}
	}

	dsn := fmt.Sprintf("file:%s?mode=ro", url.QueryEscape(path))
	if _, err := a.db.ExecContext(ctx, "ATTACH DATABASE ? AS sc", dsn); err != nil {
		a.recordError(err.Error())
		return fmt.Errorf("attach score %q: %w", path, err)
	}

	// score テーブルの存在確認も兼ねて COUNT を取る。失敗したら誤ったファイル
	// (例: songdata.db を選択してしまった場合) なので ATTACH をロールバックして
	// エラー返却する。後続クエリ ("no such table: sc.score") を防ぎ、設定画面で
	// 即座にエラーを表示できるようにする。
	var count int
	row := a.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sc.score")
	if err := row.Scan(&count); err != nil {
		msg := fmt.Sprintf("score テーブルが見つかりません (%s に score テーブルが無いか、beatoraja の score.db ではありません): %v", path, err)
		a.recordError(msg)
		if _, derr := a.db.ExecContext(ctx, "DETACH DATABASE sc"); derr != nil {
			a.log.Warn("detach after count failure also failed", "err", derr)
		}
		return fmt.Errorf("validate sc.score %q: %w", path, err)
	}

	now := a.clock.Now()
	a.mu.Lock()
	a.attached = true
	a.path = path
	a.rowCount = count
	a.attachedAt = &now
	a.lastErr = ""
	a.mu.Unlock()
	a.log.Info("score attached", "path", path, "count", count)
	return nil
}

// Detach は schema 'sc' を DETACH する。未アタッチなら no-op。
func (a *ScoreDBAttacher) Detach(ctx context.Context) error {
	if !a.IsAttached() {
		return nil
	}
	if _, err := a.db.ExecContext(ctx, "DETACH DATABASE sc"); err != nil {
		return fmt.Errorf("detach score: %w", err)
	}
	a.mu.Lock()
	a.attached = false
	a.path = ""
	a.rowCount = 0
	a.attachedAt = nil
	a.mu.Unlock()
	a.log.Info("score detached")
	return nil
}

// ReAttach は Detach → Attach を 1 連の操作で行う (設定変更時のフック用)。
// path が空のときは Detach のみ行う。
func (a *ScoreDBAttacher) ReAttach(ctx context.Context, path string) error {
	if err := a.Detach(ctx); err != nil {
		return err
	}
	return a.Attach(ctx, path)
}

// IsAttached は現在 'sc' がアタッチされているかを返す。
func (a *ScoreDBAttacher) IsAttached() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.attached
}

func (a *ScoreDBAttacher) recordError(msg string) {
	a.mu.Lock()
	a.lastErr = msg
	a.mu.Unlock()
}
