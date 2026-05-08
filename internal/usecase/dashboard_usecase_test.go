package usecase_test

import (
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"

	"github.com/stretchr/testify/assert"
)

func TestDashboardUseCase_AppendAndSnapshot(t *testing.T) {
	t.Parallel()
	pickStore := usecase.NewPickResultStore()
	d := usecase.NewDashboardUseCase(pickStore)

	now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)
	d.AppendRequest(domain.RequestLogEntry{At: now, Method: "GET", Path: "/sl-random", StatusCode: 200, DurationMs: 12, Slug: "sl-random"})
	d.AppendRequest(domain.RequestLogEntry{At: now.Add(time.Second), Method: "GET", Path: "/sl-random/data.json", StatusCode: 200, DurationMs: 4, Slug: "sl-random"})
	d.AppendFetch(domain.FetchLogEntry{At: now, SourceID: "src1", DisplayName: "Satellite", Status: domain.FetchStatusOK})

	pickStore.Set("pub1", domain.PickResult{
		PublishedTableID: "pub1",
		GeneratedAt:      now,
		LevelOrder:       []string{"sl0", "sl1"},
		Charts: []domain.SourceChart{
			{Level: "sl0"}, {Level: "sl0"}, {Level: "sl1"},
		},
	})

	snap := d.Snapshot()
	assert.Len(t, snap.Requests, 2)
	assert.Equal(t, "/sl-random/data.json", snap.Requests[0].Path, "newest first")
	assert.Len(t, snap.Fetches, 1)
	assert.Len(t, snap.Picks, 1)
	assert.Equal(t, "pub1", snap.Picks[0].PublishedID)
	assert.Equal(t, 3, snap.Picks[0].TotalCount)
	assert.Equal(t, map[string]int{"sl0": 2, "sl1": 1}, snap.Picks[0].LevelCounts)
}

func TestDashboardUseCase_RingBufferCapacity(t *testing.T) {
	t.Parallel()
	pickStore := usecase.NewPickResultStore()
	d := usecase.NewDashboardUseCase(pickStore)
	for i := 0; i < 150; i++ {
		d.AppendRequest(domain.RequestLogEntry{Method: "GET", Path: "/x", StatusCode: 200})
	}
	snap := d.Snapshot()
	assert.Len(t, snap.Requests, 100, "capped at 100")
}
