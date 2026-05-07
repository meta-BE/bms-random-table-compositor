package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"

	_ "modernc.org/sqlite"
)

// SongdataReader は beatoraja の songdata.db から所持 md5 集合を読む port.OwnedChartRepo 実装。
type SongdataReader struct{}

// NewSongdataReader は新しい SongdataReader を作る。
func NewSongdataReader() *SongdataReader {
	return &SongdataReader{}
}

// LoadOwnedMD5Set は dbPath を read-only で開き、song.md5 を読み出して集合で返す。
//
// dbPath が空文字列の場合は空 set + error なしで返す（spec §8: 「DB 未設定時は owned_only の表は 0 件」と整合）。
// dbPath が存在しないファイルなら明示的にエラー（GUI で「ファイルが見つかりません」と表示できるように）。
func (r *SongdataReader) LoadOwnedMD5Set(ctx context.Context, dbPath string) (map[string]struct{}, error) {
	if dbPath == "" {
		return map[string]struct{}{}, nil
	}
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("songdata.db を開けません %q: %w", dbPath, err)
	}

	// modernc/sqlite の DSN: クエリパラメータで mode=ro と _busy_timeout を指定。
	// パスは url.QueryEscape して特殊文字（スペース・日本語）に対応。
	dsn := fmt.Sprintf("file:%s?mode=ro&_busy_timeout=2000", url.QueryEscape(dbPath))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite open ro %q: %w", dbPath, err)
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `SELECT md5 FROM song`)
	if err != nil {
		return nil, fmt.Errorf("select md5: %w", err)
	}
	defer rows.Close()

	out := make(map[string]struct{}, 4096)
	for rows.Next() {
		var md5 string
		if err := rows.Scan(&md5); err != nil {
			return nil, fmt.Errorf("scan md5: %w", err)
		}
		if md5 != "" {
			out[md5] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows err: %w", err)
	}
	return out, nil
}
