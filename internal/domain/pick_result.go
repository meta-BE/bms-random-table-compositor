package domain

import "time"

// PickResult はピック結果。in-memory 揮発（プロセス再起動で消える）。
// SeedKey は daily モードのキャッシュ判定に使う：今日の YYYY-MM-DD と一致したら再生成不要。
type PickResult struct {
	PublishedTableID string
	GeneratedAt      time.Time
	SeedKey          string        // per_request: nano 値の文字列、daily: YYYY-MM-DD、manual: 手動更新時刻 ISO8601
	Charts           []SourceChart // ピック後・整列済み（レベル間 / レベル内）
	LevelOrder       []string      // 1 曲以上残ったレベルのみ抽出済み（応答 header.json で使う）
}
