package domain

import "time"

// EnrichedChart は SourceChart にローカル DB 由来の状態を載せた読み取り専用ビュー。
// 永続化はせず、リクエスト毎に SourceTableRepo.LoadCharts が SQL で組み立てる。
type EnrichedChart struct {
	SourceChart             // 既存フィールドを埋め込み
	IsOwned      bool       // sd.song に存在するか (未アタッチ時は false)
	LastPlayedAt *time.Time // sd.score 由来。実取得は v2、現状は常に nil
}
