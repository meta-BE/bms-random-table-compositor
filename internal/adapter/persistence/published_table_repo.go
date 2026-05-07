package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

// PublishedTableRepoSQL は published_table の永続化を担う port.PublishedTableRepo 実装。
type PublishedTableRepoSQL struct {
	db *sql.DB
}

// NewPublishedTableRepoSQL は新しい PublishedTableRepoSQL を作る。
func NewPublishedTableRepoSQL(db *sql.DB) *PublishedTableRepoSQL {
	return &PublishedTableRepoSQL{db: db}
}

const publishedTableSelectColumns = `SELECT
	id, slug, display_name, symbol, source_table_id, owned_only,
	pick_per_level, pick_refresh_mode, prefer_old_play, sort_order
 FROM published_table`

func (r *PublishedTableRepoSQL) scanRow(s rowScanner) (domain.PublishedTable, error) {
	var (
		t         domain.PublishedTable
		ownedOnly int
		preferOld int
		mode      string
	)
	if err := s.Scan(
		&t.ID, &t.Slug, &t.DisplayName, &t.Symbol, &t.SourceTableID, &ownedOnly,
		&t.Pick.PerLevel, &mode, &preferOld, &t.SortOrder,
	); err != nil {
		return domain.PublishedTable{}, err
	}
	t.OwnedOnly = ownedOnly != 0
	t.Pick.PreferOldPlay = preferOld != 0
	t.Pick.RefreshMode = domain.RefreshMode(mode)
	return t, nil
}

// Create は PublishedTable を新規挿入する。slug の UNIQUE 違反は ErrSlugDuplicated を返す。
func (r *PublishedTableRepoSQL) Create(ctx context.Context, t domain.PublishedTable) (string, error) {
	if t.ID == "" {
		return "", errors.New("ID は必須です")
	}
	owned := 0
	if t.OwnedOnly {
		owned = 1
	}
	preferOld := 0
	if t.Pick.PreferOldPlay {
		preferOld = 1
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO published_table
		 (id, slug, display_name, symbol, source_table_id, owned_only,
		  pick_per_level, pick_refresh_mode, prefer_old_play, sort_order)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Slug, t.DisplayName, t.Symbol, t.SourceTableID, owned,
		t.Pick.PerLevel, string(t.Pick.RefreshMode), preferOld, t.SortOrder,
	)
	if err != nil {
		if isUniqueSlugViolation(err) {
			return "", fmt.Errorf("%w: %s", usecase.ErrSlugDuplicated, t.Slug)
		}
		return "", fmt.Errorf("insert published_table %q: %w", t.ID, err)
	}
	return t.ID, nil
}

// isUniqueSlugViolation は modernc/sqlite が返す UNIQUE 制約違反かを判定する。
// modernc は標準 SQLite の "UNIQUE constraint failed: published_table.slug" メッセージを保つ。
func isUniqueSlugViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") &&
		strings.Contains(msg, "published_table.slug")
}

// Get は ID で取得する。存在しない場合は ErrPublishedTableNotFound を返す。
func (r *PublishedTableRepoSQL) Get(ctx context.Context, id string) (domain.PublishedTable, error) {
	row := r.db.QueryRowContext(ctx, publishedTableSelectColumns+` WHERE id = ?`, id)
	t, err := r.scanRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PublishedTable{}, usecase.ErrPublishedTableNotFound
	}
	if err != nil {
		return domain.PublishedTable{}, fmt.Errorf("get published_table %q: %w", id, err)
	}
	return t, nil
}

// GetBySlug は slug で取得する。存在しない場合は ErrPublishedTableNotFound を返す。
func (r *PublishedTableRepoSQL) GetBySlug(ctx context.Context, slug string) (domain.PublishedTable, error) {
	row := r.db.QueryRowContext(ctx, publishedTableSelectColumns+` WHERE slug = ?`, slug)
	t, err := r.scanRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PublishedTable{}, usecase.ErrPublishedTableNotFound
	}
	if err != nil {
		return domain.PublishedTable{}, fmt.Errorf("get published_table by slug %q: %w", slug, err)
	}
	return t, nil
}

// List は sort_order, created_at 順に返す。
func (r *PublishedTableRepoSQL) List(ctx context.Context) ([]domain.PublishedTable, error) {
	rows, err := r.db.QueryContext(ctx,
		publishedTableSelectColumns+` ORDER BY sort_order ASC, created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list published_table: %w", err)
	}
	defer rows.Close()
	var out []domain.PublishedTable
	for rows.Next() {
		t, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Update は値を上書きする。slug の UNIQUE 違反は ErrSlugDuplicated を返す。
func (r *PublishedTableRepoSQL) Update(ctx context.Context, t domain.PublishedTable) error {
	owned := 0
	if t.OwnedOnly {
		owned = 1
	}
	preferOld := 0
	if t.Pick.PreferOldPlay {
		preferOld = 1
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE published_table SET
		   slug=?, display_name=?, symbol=?, source_table_id=?, owned_only=?,
		   pick_per_level=?, pick_refresh_mode=?, prefer_old_play=?, sort_order=?,
		   updated_at=datetime('now')
		 WHERE id=?`,
		t.Slug, t.DisplayName, t.Symbol, t.SourceTableID, owned,
		t.Pick.PerLevel, string(t.Pick.RefreshMode), preferOld, t.SortOrder, t.ID,
	)
	if err != nil {
		if isUniqueSlugViolation(err) {
			return fmt.Errorf("%w: %s", usecase.ErrSlugDuplicated, t.Slug)
		}
		return fmt.Errorf("update published_table %q: %w", t.ID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return usecase.ErrPublishedTableNotFound
	}
	return nil
}

// Delete は ID で削除する。存在しなくてもエラーにしない（冪等）。
func (r *PublishedTableRepoSQL) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM published_table WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("delete published_table %q: %w", id, err)
	}
	return nil
}

// SlugExists は slug が既に使われているかを返す。excludeID を指定すると自分自身は除外する。
func (r *PublishedTableRepoSQL) SlugExists(ctx context.Context, slug string, excludeID string) (bool, error) {
	var count int
	var err error
	if excludeID == "" {
		err = r.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM published_table WHERE slug = ?`, slug).Scan(&count)
	} else {
		err = r.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM published_table WHERE slug = ? AND id <> ?`,
			slug, excludeID).Scan(&count)
	}
	if err != nil {
		return false, fmt.Errorf("slug exists %q: %w", slug, err)
	}
	return count > 0, nil
}
