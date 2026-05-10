package domain

// PublishedTableLevel は公開表が持つ 1 つの公開レベル。
// PerMappingPick (m) は「各マッピングからの最低保証ピック数」、
// TotalPick (n) は「公開レベル全体の目標合計ピック数」。
// 詳細は docs/superpowers/specs/2026-05-10-multi-source-table-composition-design.md §3 参照。
type PublishedTableLevel struct {
	ID               string
	PublishedTableID string
	Name             string // 公開レベル表示名（例: "5", "Lv.5", "中級"）
	SortOrder        int
	PerMappingPick   int // m: 各マッピングからの最低保証ピック数 (>= 0)
	TotalPick        int // n: 公開レベル全体の目標合計ピック数 (>= 0)
	Mappings         []PublishedTableLevelMapping // SortOrder 昇順
}

// PublishedTableLevelMapping は公開レベルが参照する 1 件のソース表レベル。
type PublishedTableLevelMapping struct {
	ID                    string
	PublishedTableLevelID string
	SourceTableID         string
	SourceLevel           string // ソース表内のレベル文字列（例: "5", "★5"）
	SortOrder             int
}
