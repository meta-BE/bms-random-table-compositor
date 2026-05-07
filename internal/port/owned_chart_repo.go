package port

import "context"

// OwnedChartRepo は beatoraja の songdata.db から所持譜面の md5 集合を取得する。
type OwnedChartRepo interface {
	// LoadOwnedMD5Set は dbPath を read-only で開き、song.md5 を読み出して集合で返す。
	// dbPath が空のときは空 set を error なしで返す（spec §8 の「未設定 = 0 件」と整合）。
	LoadOwnedMD5Set(ctx context.Context, dbPath string) (map[string]struct{}, error)
}
