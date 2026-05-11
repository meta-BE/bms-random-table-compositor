package domain

// RefreshMode は公開表のピック更新モード。
type RefreshMode string

const (
	RefreshModePerRequest RefreshMode = "per_request"
	RefreshModeDaily      RefreshMode = "daily"
	RefreshModeManual     RefreshMode = "manual"
)

// WeightMode は重み付けピックのモード。
type WeightMode string

const (
	WeightModeOff         WeightMode = "off"         // 一様ランダム
	WeightModeProbability WeightMode = "probability" // 確率 (X 倍まで偏らせる)
	WeightModeSort        WeightMode = "sort"        // 完全日時順ソート (古い順)
)

// PickConfig はピック生成に必要な設定値。
// PerLevel / PreferOldPlay は撤去（複数ソース表合成スペックで Levels[].PerMappingPick/TotalPick と Weighter に置き換わった）。
type PickConfig struct {
	RefreshMode  RefreshMode // per_request / daily / manual
	WeightMode   WeightMode  // 既定 WeightModeOff
	WeightParamX int         // 既定 10、probability モードでのみ使用
}

// PublishedTable はユーザーが公開する表。Levels に複数ソース表のレベルを合成して持つ。
type PublishedTable struct {
	ID          string
	Slug        string
	DisplayName string
	Symbol      string
	OwnedOnly   bool
	Pick        PickConfig
	SortOrder   int
	Levels      []PublishedTableLevel // SortOrder 昇順
}
