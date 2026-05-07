package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

// headerJSON は GET /{slug}/header.json のレスポンス。
type headerJSON struct {
	Name       string   `json:"name"`
	Symbol     string   `json:"symbol"`
	DataURL    string   `json:"data_url"`
	LevelOrder []string `json:"level_order"`
}

// newHeaderHandler は GET /{slug}/header.json ハンドラ。
func newHeaderHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		result, pub, err := deps.Pick.PickBySlug(r.Context(), slug)
		if err != nil {
			handleJSONError(w, err)
			return
		}
		out := headerJSON{
			Name:       pub.DisplayName,
			Symbol:     pub.Symbol,
			DataURL:    "data.json",
			LevelOrder: result.LevelOrder,
		}
		// JSON encoder は nil スライスを null にしてしまうため空配列に正規化。
		if out.LevelOrder == nil {
			out.LevelOrder = []string{}
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if err := json.NewEncoder(w).Encode(out); err != nil {
			deps.Log.Error("header.json encode failed", "slug", slug, "err", err)
		}
	}
}

// handleJSONError は usecase の sentinel error を JSON エラー応答に変換する。
func handleJSONError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, usecase.ErrPublishedTableNotFound):
		writeJSONError(w, http.StatusNotFound, "not_found")
	case errors.Is(err, usecase.ErrSourceNotFetched):
		writeJSONError(w, http.StatusServiceUnavailable, "source_not_fetched")
	default:
		writeJSONError(w, http.StatusInternalServerError, err.Error())
	}
}

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
