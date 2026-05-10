package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
)

// SourceTableRepoSQL は source_table / source_table_chart を扱う port.SourceTableRepo の実装。
type SourceTableRepoSQL struct {
	db       *sql.DB
	attacher *SongdataAttacher
}

// NewSourceTableRepoSQL は新しい SourceTableRepoSQL を作る。
// attacher 経由で songdata.db のアタッチ状態を見て LoadCharts の SQL を切り替える。
func NewSourceTableRepoSQL(db *sql.DB, attacher *SongdataAttacher) *SourceTableRepoSQL {
	return &SourceTableRepoSQL{db: db, attacher: attacher}
}

// Create は SourceTable を新規挿入し、ID を返す。
// CHECK 制約に引っかからないよう InputKind / LastFetchStatus はゼロ値時に既定値を補完する。
func (r *SourceTableRepoSQL) Create(ctx context.Context, in domain.SourceTable) (string, error) {
	if in.ID == "" {
		return "", errors.New("ID は必須です")
	}
	kind := in.InputKind
	if kind == "" {
		kind = domain.InputKindHeaderJSON
	}
	status := in.LastFetchStatus
	if status == "" {
		status = domain.FetchStatusNever
	}
	levelOrderJSON, err := json.Marshal(in.LevelOrder)
	if err != nil {
		return "", fmt.Errorf("marshal level_order: %w", err)
	}
	if string(levelOrderJSON) == "null" {
		levelOrderJSON = []byte("[]")
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO source_table
		 (id, input_url, input_kind, display_name, name, symbol, level_order_json,
		  data_url, etag, last_fetched_at, last_fetch_status, last_fetch_error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.ID, in.InputURL, string(kind), in.DisplayName, in.Name, in.Symbol,
		string(levelOrderJSON), in.DataURL, in.ETag,
		fetchedAtToNullable(in.LastFetchedAt), string(status), in.LastFetchError,
	)
	if err != nil {
		return "", fmt.Errorf("insert source_table %q: %w", in.ID, err)
	}
	return in.ID, nil
}

// Get は ID で SourceTable を取得する。存在しない場合はエラー。
func (r *SourceTableRepoSQL) Get(ctx context.Context, id string) (domain.SourceTable, error) {
	row := r.db.QueryRowContext(ctx, sourceTableSelectColumns+` WHERE id = ?`, id)
	st, err := scanSourceTable(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.SourceTable{}, fmt.Errorf("source_table %q が見つかりません", id)
	}
	if err != nil {
		return domain.SourceTable{}, err
	}
	return st, nil
}

// List は sort_order, created_at 順に SourceTable を返す。
func (r *SourceTableRepoSQL) List(ctx context.Context) ([]domain.SourceTable, error) {
	rows, err := r.db.QueryContext(ctx,
		sourceTableSelectColumns+` ORDER BY sort_order ASC, created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list source_table: %w", err)
	}
	defer rows.Close()
	var out []domain.SourceTable
	for rows.Next() {
		st, err := scanSourceTable(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// Update は SourceTable のメタ情報を上書きする。fetched 系カラムも上書きするので、
// 通常用途以外で呼ぶ場合は最新の値を読み出してから書き戻すこと。
func (r *SourceTableRepoSQL) Update(ctx context.Context, t domain.SourceTable) error {
	levelOrderJSON, err := json.Marshal(t.LevelOrder)
	if err != nil {
		return fmt.Errorf("marshal level_order: %w", err)
	}
	if string(levelOrderJSON) == "null" {
		levelOrderJSON = []byte("[]")
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE source_table SET
		   input_url=?, input_kind=?, display_name=?, name=?, symbol=?,
		   level_order_json=?, data_url=?, etag=?, last_fetched_at=?,
		   last_fetch_status=?, last_fetch_error=?, updated_at=datetime('now')
		 WHERE id=?`,
		t.InputURL, string(t.InputKind), t.DisplayName, t.Name, t.Symbol,
		string(levelOrderJSON), t.DataURL, t.ETag, fetchedAtToNullable(t.LastFetchedAt),
		string(t.LastFetchStatus), t.LastFetchError, t.ID,
	)
	if err != nil {
		return fmt.Errorf("update source_table %q: %w", t.ID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("source_table %q が見つかりません", t.ID)
	}
	return nil
}

// Delete は ID で行を削除する。存在しなくてもエラーにしない（冪等）。
func (r *SourceTableRepoSQL) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM source_table WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("delete source_table %q: %w", id, err)
	}
	return nil
}

// ---- ヘルパ ----

const sourceTableSelectColumns = `SELECT
	id, input_url, input_kind, display_name, name, symbol, level_order_json,
	data_url, etag, last_fetched_at, last_fetch_status, last_fetch_error
 FROM source_table`

// scanSourceTable は *sql.Row / *sql.Rows どちらでも使えるように Scanner で受ける。
type rowScanner interface {
	Scan(dest ...any) error
}

func scanSourceTable(s rowScanner) (domain.SourceTable, error) {
	var (
		st              domain.SourceTable
		levelOrderJSON  string
		lastFetchedAt   sql.NullString
		lastFetchStatus string
		inputKind       string
	)
	if err := s.Scan(
		&st.ID, &st.InputURL, &inputKind, &st.DisplayName, &st.Name, &st.Symbol,
		&levelOrderJSON, &st.DataURL, &st.ETag, &lastFetchedAt,
		&lastFetchStatus, &st.LastFetchError,
	); err != nil {
		return domain.SourceTable{}, err
	}
	st.InputKind = domain.InputKind(inputKind)
	st.LastFetchStatus = domain.FetchStatus(lastFetchStatus)
	if levelOrderJSON != "" && levelOrderJSON != "null" {
		if err := json.Unmarshal([]byte(levelOrderJSON), &st.LevelOrder); err != nil {
			return domain.SourceTable{}, fmt.Errorf("unmarshal level_order_json: %w", err)
		}
	}
	if lastFetchedAt.Valid && lastFetchedAt.String != "" {
		t, err := time.Parse(time.RFC3339, lastFetchedAt.String)
		if err == nil {
			st.LastFetchedAt = &t
		}
	}
	return st, nil
}

func fetchedAtToNullable(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

// SaveFetched は取得結果を Tx 内で保存する。
// NotModified=true の場合は last_fetched_at / updated_at のみ更新し、譜面行は触らない。
func (r *SourceTableRepoSQL) SaveFetched(
	ctx context.Context, sourceID string, ft port.FetchedTable, fetchedAt time.Time,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	fetchedAtStr := fetchedAt.UTC().Format(time.RFC3339)

	if ft.NotModified {
		_, err = tx.ExecContext(ctx,
			`UPDATE source_table SET
			   last_fetched_at=?, last_fetch_status='ok', last_fetch_error='',
			   updated_at=datetime('now')
			 WHERE id=?`,
			fetchedAtStr, sourceID,
		)
		if err != nil {
			return fmt.Errorf("update source_table (not_modified) %q: %w", sourceID, err)
		}
		return tx.Commit()
	}

	// header.json に level_order が無いソース表（例: 一部の satellite/stella）に対しては、
	// charts から自然順で導出した値で埋める。空のままだとウィザードや
	// マッピング編集 UI が「レベル選択肢ゼロ」になり機能停止する (両バグの fix)。
	levelOrder := ft.Header.LevelOrder
	if len(levelOrder) == 0 {
		levelOrder = deriveLevelOrderFromCharts(ft.Charts)
	}
	levelOrderJSON, err := json.Marshal(levelOrder)
	if err != nil {
		return fmt.Errorf("marshal level_order: %w", err)
	}
	if string(levelOrderJSON) == "null" {
		levelOrderJSON = []byte("[]")
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE source_table SET
		   name=?, symbol=?, level_order_json=?, data_url=?, etag=?,
		   last_fetched_at=?, last_fetch_status='ok', last_fetch_error='',
		   updated_at=datetime('now')
		 WHERE id=?`,
		ft.Header.Name, ft.Header.Symbol, string(levelOrderJSON),
		ft.Header.DataURL, ft.ETag, fetchedAtStr, sourceID,
	)
	if err != nil {
		return fmt.Errorf("update source_table %q: %w", sourceID, err)
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM source_table_chart WHERE source_id=?`, sourceID); err != nil {
		return fmt.Errorf("delete charts %q: %w", sourceID, err)
	}

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO source_table_chart
		 (source_id, position, md5, sha256, level, title, artist, raw_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare insert chart: %w", err)
	}
	defer stmt.Close()

	for _, c := range ft.Charts {
		rawJSON, err := json.Marshal(c.Raw)
		if err != nil {
			return fmt.Errorf("marshal raw[pos=%d]: %w", c.Position, err)
		}
		if _, err := stmt.ExecContext(ctx,
			sourceID, c.Position, c.MD5, c.SHA256, c.Level, c.Title, c.Artist, string(rawJSON),
		); err != nil {
			return fmt.Errorf("insert chart[pos=%d]: %w", c.Position, err)
		}
	}
	return tx.Commit()
}

// LoadCharts は source_table_chart を position 昇順で EnrichedChart として返す。
// SongdataAttacher が sd をアタッチ済みなら IsOwned を EXISTS sd.song で計算する。
// 未アタッチ時は IsOwned=false で返し、q.OwnedOnly=true なら空配列を返す
// (spec: DB 未設定時は owned_only の表は 0 件)。
// LastPlayedAt は今回 NULL 固定 (v2 で sd.score を見る形に拡張する)。
func (r *SourceTableRepoSQL) LoadCharts(
	ctx context.Context, sourceID string, q port.ChartQuery,
) ([]domain.EnrichedChart, error) {
	if r.attacher != nil && r.attacher.IsAttached() {
		return r.loadChartsAttached(ctx, sourceID, q)
	}
	if q.OwnedOnly {
		return nil, nil
	}
	return r.loadChartsBare(ctx, sourceID)
}

func (r *SourceTableRepoSQL) loadChartsAttached(
	ctx context.Context, sourceID string, q port.ChartQuery,
) ([]domain.EnrichedChart, error) {
	ownedFlag := 0
	if q.OwnedOnly {
		ownedFlag = 1
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT
		  c.position, t.symbol, c.md5, c.sha256, c.level, c.title, c.artist, c.raw_json,
		  EXISTS(SELECT 1 FROM sd.song s WHERE s.md5 = c.md5)        AS is_owned,
		  NULL                                                        AS last_played_at
		FROM source_table_chart c
		JOIN source_table t ON t.id = c.source_id
		WHERE c.source_id = ?
		  AND (? = 0 OR EXISTS (SELECT 1 FROM sd.song s WHERE s.md5 = c.md5))
		ORDER BY c.position ASC`,
		sourceID, ownedFlag,
	)
	if err != nil {
		return nil, fmt.Errorf("load enriched charts (attached) %q: %w", sourceID, err)
	}
	defer rows.Close()
	return scanEnrichedRows(rows, sourceID)
}

func (r *SourceTableRepoSQL) loadChartsBare(
	ctx context.Context, sourceID string,
) ([]domain.EnrichedChart, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.position, t.symbol, c.md5, c.sha256, c.level, c.title, c.artist, c.raw_json,
		       0 AS is_owned, NULL AS last_played_at
		FROM source_table_chart c
		JOIN source_table t ON t.id = c.source_id
		WHERE c.source_id = ?
		ORDER BY c.position ASC`,
		sourceID,
	)
	if err != nil {
		return nil, fmt.Errorf("load enriched charts (bare) %q: %w", sourceID, err)
	}
	defer rows.Close()
	return scanEnrichedRows(rows, sourceID)
}

func scanEnrichedRows(rows *sql.Rows, sourceID string) ([]domain.EnrichedChart, error) {
	var out []domain.EnrichedChart
	for rows.Next() {
		var (
			c            domain.SourceChart
			rawJSON      string
			isOwned      bool
			lastPlayedAt sql.NullString // 現状は常に NULL、v2 でカラム化予定
		)
		if err := rows.Scan(
			&c.Position, &c.Symbol, &c.MD5, &c.SHA256, &c.Level, &c.Title, &c.Artist,
			&rawJSON, &isOwned, &lastPlayedAt,
		); err != nil {
			return nil, err
		}
		c.SourceID = sourceID
		if rawJSON != "" {
			if err := json.Unmarshal([]byte(rawJSON), &c.Raw); err != nil {
				return nil, fmt.Errorf("unmarshal raw_json[pos=%d]: %w", c.Position, err)
			}
		}
		ec := domain.EnrichedChart{SourceChart: c, IsOwned: isOwned}
		// lastPlayedAt は今回常に NULL のため Valid=false → ec.LastPlayedAt は nil
		_ = lastPlayedAt
		out = append(out, ec)
	}
	return out, rows.Err()
}

// deriveLevelOrderFromCharts は charts から distinct level を自然順で並べる。
// 数値解釈できるレベル（"1", "2", "1.5" 等）を数値昇順で先に置き、
// 数値解釈できない文字列（"段位1", "?" 等）を文字列昇順で末尾に置く。
// header.json に level_order が無いソース表（例: 一部の satellite/stella）に対して、
// LevelOrder を意味のある値で埋めるために SaveFetched から呼ばれる。
func deriveLevelOrderFromCharts(charts []domain.SourceChart) []string {
	seen := map[string]struct{}{}
	var levels []string
	for _, c := range charts {
		if c.Level == "" {
			continue
		}
		if _, ok := seen[c.Level]; ok {
			continue
		}
		seen[c.Level] = struct{}{}
		levels = append(levels, c.Level)
	}
	sort.SliceStable(levels, func(i, j int) bool {
		ai, aok := parseLevelNumeric(levels[i])
		bj, bok := parseLevelNumeric(levels[j])
		if aok != bok {
			return aok // 数値解釈できる方が先
		}
		if aok && ai != bj {
			return ai < bj
		}
		return levels[i] < levels[j]
	})
	return levels
}

func parseLevelNumeric(s string) (float64, bool) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// MarkFetchError は取得失敗を記録する。譜面行は触らない（前回成功時のキャッシュを保持）。
func (r *SourceTableRepoSQL) MarkFetchError(
	ctx context.Context, sourceID string, fetchErr error, fetchedAt time.Time,
) error {
	msg := ""
	if fetchErr != nil {
		msg = fetchErr.Error()
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE source_table SET
		   last_fetched_at=?, last_fetch_status='error', last_fetch_error=?,
		   updated_at=datetime('now')
		 WHERE id=?`,
		fetchedAt.UTC().Format(time.RFC3339), msg, sourceID,
	)
	if err != nil {
		return fmt.Errorf("mark fetch error %q: %w", sourceID, err)
	}
	return nil
}
