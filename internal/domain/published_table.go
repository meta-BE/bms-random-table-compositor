package domain

// RefreshMode は公開表のピック更新モード。
type RefreshMode string

const (
	RefreshModePerRequest RefreshMode = "per_request"
	RefreshModeDaily      RefreshMode = "daily"
	RefreshModeManual     RefreshMode = "manual"
)

// PickConfig はピック生成に必要な設定値。
type PickConfig struct {
	PerLevel      int         // 0 = 無制限（全件返す）
	RefreshMode   RefreshMode // per_request / daily / manual
	PreferOldPlay bool        // v2 用フラグ。Plan 3 では未使用（常に false）
}

// PublishedTable はユーザーが公開する表。1 公開表 = 1 ソース表（合成は v2）。
type PublishedTable struct {
	ID            string
	Slug          string
	DisplayName   string
	Symbol        string
	SourceTableID string
	OwnedOnly     bool
	Pick          PickConfig
	SortOrder     int
}
