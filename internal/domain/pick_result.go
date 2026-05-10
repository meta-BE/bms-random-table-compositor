package domain

import "time"

// PickedChart は EnrichedChart にピック時の公開レベル名を併記した結果。
// EnrichedChart.Level はソース表側のレベル（HTML 行頭セルで Symbol と組み合わせて表示）。
// PublicLevel は公開表側で割り当てられたレベル名（data.json の level フィールド出力 +
// HTML のグルーピング + ダッシュボードのレベル別カウントで使う）。
type PickedChart struct {
	EnrichedChart        // ソース由来のフィールドを埋め込み
	PublicLevel   string // 公開レベル名（HTML/data.json/ダッシュボードで使う）
}

// PickResult はピック結果。in-memory 揮発（プロセス再起動で消える）。
// SeedKey は daily モードのキャッシュ判定に使う：今日の YYYY-MM-DD と一致したら再生成不要。
type PickResult struct {
	PublishedTableID string
	GeneratedAt      time.Time
	SeedKey          string        // per_request: nano 値の文字列、daily: YYYY-MM-DD、manual: 手動更新時刻 ISO8601
	Charts           []PickedChart // ピック後・整列済み (IsOwned/LastPlayedAt 付与)
	LevelOrder       []string      // 公開レベル名の並び順（ピック結果が 0 件のレベルを除外済み）
}
