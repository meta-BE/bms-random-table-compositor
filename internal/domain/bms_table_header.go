package domain

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// BMSTableHeader は header.json をデコードした構造体。
// `level_order` は string 配列 / 数値配列の両方を受け付け、内部表現は []string に正規化する。
type BMSTableHeader struct {
	Name       string
	Symbol     string
	DataURL    string
	LevelOrder []string
}

type rawBMSTableHeader struct {
	Name       string          `json:"name"`
	Symbol     string          `json:"symbol"`
	DataURL    string          `json:"data_url"`
	LevelOrder json.RawMessage `json:"level_order"`
}

// UnmarshalJSON は level_order の型ゆらぎ（string 配列 / number 配列）を吸収する。
func (h *BMSTableHeader) UnmarshalJSON(data []byte) error {
	var raw rawBMSTableHeader
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	h.Name = raw.Name
	h.Symbol = raw.Symbol
	h.DataURL = raw.DataURL

	if len(raw.LevelOrder) == 0 || string(raw.LevelOrder) == "null" {
		h.LevelOrder = nil
		return nil
	}

	var asStrings []string
	if err := json.Unmarshal(raw.LevelOrder, &asStrings); err == nil {
		h.LevelOrder = asStrings
		return nil
	}
	var asNumbers []float64
	if err := json.Unmarshal(raw.LevelOrder, &asNumbers); err == nil {
		out := make([]string, len(asNumbers))
		for i, n := range asNumbers {
			out[i] = strconv.FormatFloat(n, 'f', -1, 64)
		}
		h.LevelOrder = out
		return nil
	}
	return fmt.Errorf("level_order: 文字列配列または数値配列でなければなりません: %s", string(raw.LevelOrder))
}
