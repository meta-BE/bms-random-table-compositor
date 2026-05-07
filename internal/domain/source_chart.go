package domain

// SourceChart はソース表の譜面エントリ。`Raw` には data.json の元エントリを
// パススルー保持する（HTTP 応答時に表固有フィールドをそのまま返すため）。
type SourceChart struct {
	SourceID string
	Position int
	MD5      string
	SHA256   string
	Level    string
	Title    string
	Artist   string
	Raw      map[string]any
}
