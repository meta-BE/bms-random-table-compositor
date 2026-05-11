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

// PublishedTableRepoSQL は published_table / _level / _level_mapping を一括で扱う実装。
type PublishedTableRepoSQL struct {
	db *sql.DB
}

func NewPublishedTableRepoSQL(db *sql.DB) *PublishedTableRepoSQL {
	return &PublishedTableRepoSQL{db: db}
}

const publishedTableSelectColumns = `SELECT
	id, slug, display_name, symbol, owned_only,
	pick_refresh_mode, weight_mode, weight_param_x, sort_order
 FROM published_table`

func (r *PublishedTableRepoSQL) scanRow(s rowScanner) (domain.PublishedTable, error) {
	var (
		t         domain.PublishedTable
		ownedOnly int
		mode      string
		wMode     string
		wX        int
	)
	if err := s.Scan(
		&t.ID, &t.Slug, &t.DisplayName, &t.Symbol, &ownedOnly,
		&mode, &wMode, &wX, &t.SortOrder,
	); err != nil {
		return domain.PublishedTable{}, err
	}
	t.OwnedOnly = ownedOnly != 0
	t.Pick.RefreshMode = domain.RefreshMode(mode)
	t.Pick.WeightMode = domain.WeightMode(wMode)
	t.Pick.WeightParamX = wX
	return t, nil
}

// Create は PublishedTable を Levels/Mappings 込みで一括 INSERT する（1 トランザクション）。
func (r *PublishedTableRepoSQL) Create(ctx context.Context, t domain.PublishedTable) (string, error) {
	if t.ID == "" {
		return "", errors.New("ID は必須です")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	owned := 0
	if t.OwnedOnly {
		owned = 1
	}
	wMode := string(t.Pick.WeightMode)
	if wMode == "" {
		wMode = string(domain.WeightModeOff)
	}
	wX := t.Pick.WeightParamX
	if wX <= 0 {
		wX = 10
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO published_table
		 (id, slug, display_name, symbol, owned_only, pick_refresh_mode,
		  weight_mode, weight_param_x, sort_order)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Slug, t.DisplayName, t.Symbol, owned, string(t.Pick.RefreshMode),
		wMode, wX, t.SortOrder,
	); err != nil {
		if isUniqueSlugViolation(err) {
			return "", fmt.Errorf("%w: %s", usecase.ErrSlugDuplicated, t.Slug)
		}
		return "", fmt.Errorf("insert published_table %q: %w", t.ID, err)
	}
	if err := r.insertLevels(ctx, tx, t.ID, t.Levels); err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	return t.ID, nil
}

// insertLevels は published_table_id 配下の levels と mappings をまとめて INSERT する。
func (r *PublishedTableRepoSQL) insertLevels(ctx context.Context, tx *sql.Tx, pubID string, levels []domain.PublishedTableLevel) error {
	for _, lv := range levels {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO published_table_level
			 (id, published_table_id, name, sort_order, per_mapping_pick, total_pick)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			lv.ID, pubID, lv.Name, lv.SortOrder, lv.PerMappingPick, lv.TotalPick,
		); err != nil {
			return fmt.Errorf("insert level %q: %w", lv.ID, err)
		}
		for _, mp := range lv.Mappings {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO published_table_level_mapping
				 (id, published_table_level_id, source_table_id, source_level, sort_order)
				 VALUES (?, ?, ?, ?, ?)`,
				mp.ID, lv.ID, mp.SourceTableID, mp.SourceLevel, mp.SortOrder,
			); err != nil {
				return fmt.Errorf("insert mapping %q: %w", mp.ID, err)
			}
		}
	}
	return nil
}

func isUniqueSlugViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") &&
		strings.Contains(msg, "published_table.slug")
}

// Get は ID で取得する。Levels/Mappings も同時に読む。
func (r *PublishedTableRepoSQL) Get(ctx context.Context, id string) (domain.PublishedTable, error) {
	row := r.db.QueryRowContext(ctx, publishedTableSelectColumns+` WHERE id = ?`, id)
	t, err := r.scanRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PublishedTable{}, usecase.ErrPublishedTableNotFound
	}
	if err != nil {
		return domain.PublishedTable{}, fmt.Errorf("get published_table %q: %w", id, err)
	}
	if err := r.loadLevels(ctx, &t); err != nil {
		return domain.PublishedTable{}, err
	}
	return t, nil
}

func (r *PublishedTableRepoSQL) GetBySlug(ctx context.Context, slug string) (domain.PublishedTable, error) {
	row := r.db.QueryRowContext(ctx, publishedTableSelectColumns+` WHERE slug = ?`, slug)
	t, err := r.scanRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PublishedTable{}, usecase.ErrPublishedTableNotFound
	}
	if err != nil {
		return domain.PublishedTable{}, fmt.Errorf("get published_table by slug %q: %w", slug, err)
	}
	if err := r.loadLevels(ctx, &t); err != nil {
		return domain.PublishedTable{}, err
	}
	return t, nil
}

// loadLevels は対象公開表の levels と mappings を 2 クエリで取得して結合する。
func (r *PublishedTableRepoSQL) loadLevels(ctx context.Context, t *domain.PublishedTable) error {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, published_table_id, name, sort_order, per_mapping_pick, total_pick
		 FROM published_table_level
		 WHERE published_table_id = ?
		 ORDER BY sort_order ASC, id ASC`, t.ID)
	if err != nil {
		return fmt.Errorf("load levels: %w", err)
	}
	defer rows.Close()
	var levels []domain.PublishedTableLevel
	idx := map[string]int{}
	for rows.Next() {
		var lv domain.PublishedTableLevel
		if err := rows.Scan(&lv.ID, &lv.PublishedTableID, &lv.Name, &lv.SortOrder, &lv.PerMappingPick, &lv.TotalPick); err != nil {
			return fmt.Errorf("scan level: %w", err)
		}
		idx[lv.ID] = len(levels)
		levels = append(levels, lv)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(levels) == 0 {
		t.Levels = nil
		return nil
	}

	mrows, err := r.db.QueryContext(ctx,
		`SELECT m.id, m.published_table_level_id, m.source_table_id, m.source_level, m.sort_order
		 FROM published_table_level_mapping m
		 JOIN published_table_level l ON l.id = m.published_table_level_id
		 WHERE l.published_table_id = ?
		 ORDER BY m.sort_order ASC, m.id ASC`, t.ID)
	if err != nil {
		return fmt.Errorf("load mappings: %w", err)
	}
	defer mrows.Close()
	for mrows.Next() {
		var mp domain.PublishedTableLevelMapping
		if err := mrows.Scan(&mp.ID, &mp.PublishedTableLevelID, &mp.SourceTableID, &mp.SourceLevel, &mp.SortOrder); err != nil {
			return fmt.Errorf("scan mapping: %w", err)
		}
		i, ok := idx[mp.PublishedTableLevelID]
		if !ok {
			continue
		}
		levels[i].Mappings = append(levels[i].Mappings, mp)
	}
	if err := mrows.Err(); err != nil {
		return err
	}
	t.Levels = levels
	return nil
}

// List は一覧用。Levels は埋めない（軽量）。
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

// Update は子テーブル全削除 → 再 INSERT で行う（バッチ的、レコード数も小さい）。
func (r *PublishedTableRepoSQL) Update(ctx context.Context, t domain.PublishedTable) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	owned := 0
	if t.OwnedOnly {
		owned = 1
	}
	wMode := string(t.Pick.WeightMode)
	if wMode == "" {
		wMode = string(domain.WeightModeOff)
	}
	wX := t.Pick.WeightParamX
	if wX <= 0 {
		wX = 10
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE published_table SET
		   slug=?, display_name=?, symbol=?, owned_only=?,
		   pick_refresh_mode=?, weight_mode=?, weight_param_x=?,
		   sort_order=?, updated_at=datetime('now')
		 WHERE id=?`,
		t.Slug, t.DisplayName, t.Symbol, owned,
		string(t.Pick.RefreshMode), wMode, wX,
		t.SortOrder, t.ID,
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

	// 子テーブルを全削除して再 INSERT
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM published_table_level WHERE published_table_id = ?`, t.ID); err != nil {
		return fmt.Errorf("delete levels: %w", err)
	}
	// mapping は ON DELETE CASCADE で連鎖削除される
	if err := r.insertLevels(ctx, tx, t.ID, t.Levels); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// Delete は ID で削除する（Levels/Mappings は CASCADE で連鎖削除）。冪等。
func (r *PublishedTableRepoSQL) Delete(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM published_table WHERE id=?`, id); err != nil {
		return fmt.Errorf("delete published_table %q: %w", id, err)
	}
	return nil
}

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
