package handler_test

import (
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/app/handler"
	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"

	"github.com/stretchr/testify/assert"
)

func TestDashboardHandler_Snapshot(t *testing.T) {
	t.Parallel()
	pickStore := usecase.NewPickResultStore()
	uc := usecase.NewDashboardUseCase(pickStore)
	at := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)
	uc.AppendRequest(domain.RequestLogEntry{At: at, Method: "GET", Path: "/sl/data.json", StatusCode: 200, DurationMs: 5, Slug: "sl"})
	uc.AppendFetch(domain.FetchLogEntry{At: at, SourceID: "src1", DisplayName: "Satellite", Status: domain.FetchStatusOK})

	h := handler.NewDashboardHandler(uc)
	got, err := h.Snapshot()
	assert.NoError(t, err)
	assert.Len(t, got.Requests, 1)
	assert.Equal(t, "GET", got.Requests[0].Method)
	assert.Equal(t, "/sl/data.json", got.Requests[0].Path)
	assert.Equal(t, "sl", got.Requests[0].Slug)
	assert.Equal(t, 200, got.Requests[0].StatusCode)
	assert.Equal(t, int64(5), got.Requests[0].DurationMs)
	assert.Len(t, got.Fetches, 1)
	assert.Equal(t, "Satellite", got.Fetches[0].DisplayName)
}
