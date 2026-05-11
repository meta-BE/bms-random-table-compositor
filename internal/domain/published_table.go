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

// DefaultWeightParamX は WeightParamX の既定値。probability モードでの最古/最新間の重み比。
const DefaultWeightParamX = 10

// PickConfig はピック生成に必要な設定値。
// PerLevel / PreferOldPlay は撤去（複数ソース表合成スペックで Levels[].PerMappingPick/TotalPick と Weighter に置き換わった）。
type PickConfig struct {
	RefreshMode  RefreshMode // per_request / daily / manual
	WeightMode   WeightMode  // 既定 WeightModeOff
	WeightParamX int         // 既定 DefaultWeightParamX、probability モードでのみ使用
}

// NormalizedWeight は WeightMode / WeightParamX を正規化して返す。
// - 空 WeightMode は WeightModeOff
// - 0 以下の WeightParamX は DefaultWeightParamX に補完
//
// 不正な enum 値や範囲外チェックは行わない (handler 層で行う厳格な検証は別)。
// 永続化層で安全側に既定を補うための薄いヘルパ。
func NormalizedWeight(mode WeightMode, x int) (WeightMode, int) {
	if mode == "" {
		mode = WeightModeOff
	}
	if x <= 0 {
		x = DefaultWeightParamX
	}
	return mode, x
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
