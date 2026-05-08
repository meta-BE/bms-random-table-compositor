package handler_test

import (
	"context"
	"errors"
	"io/ioutil"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/app/handler"
	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	"github.com/stretchr/testify/require"
)

type sourceFakeRepo struct {
	mu   sync.Mutex
	rows map[string]domain.SourceTable
}

func newSourceFakeRepo() *sourceFakeRepo {
	return &sourceFakeRepo{rows: map[string]domain.SourceTable{}}
}
func (r *sourceFakeRepo) List(_ context.Context) ([]domain.SourceTable, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.SourceTable, 0, len(r.rows))
	for _, v := range r.rows {
		out = append(out, v)
	}
	return out, nil
}
func (r *sourceFakeRepo) Get(_ context.Context, id string) (domain.SourceTable, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.rows[id]
	if !ok {
		return domain.SourceTable{}, errors.New("not found")
	}
	return v, nil
}
func (r *sourceFakeRepo) Create(_ context.Context, in domain.SourceTable) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows[in.ID] = in
	return in.ID, nil
}
func (r *sourceFakeRepo) Update(_ context.Context, t domain.SourceTable) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows[t.ID] = t
	return nil
}
func (r *sourceFakeRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rows, id)
	return nil
}
func (r *sourceFakeRepo) SaveFetched(_ context.Context, _ string, _ port.FetchedTable, _ time.Time) error {
	return nil
}
func (r *sourceFakeRepo) MarkFetchError(_ context.Context, _ string, _ error, _ time.Time) error {
	return nil
}
func (r *sourceFakeRepo) LoadCharts(_ context.Context, _ string, _ port.ChartQuery) ([]domain.EnrichedChart, error) {
	return nil, nil
}

type sourceFakeFetcher struct{}

func (sourceFakeFetcher) FetchByHTML(_ context.Context, _ string, _ string) (port.FetchedTable, error) {
	return port.FetchedTable{}, nil
}
func (sourceFakeFetcher) FetchByHeader(_ context.Context, _ string, _ string) (port.FetchedTable, error) {
	return port.FetchedTable{}, nil
}

type sourceFakeIDGen struct{ next string }

func (g *sourceFakeIDGen) New() string { return g.next }

func newSilentHandlerLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(ioutil.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func newSourceTableHandler(t *testing.T) (*handler.SourceTableHandler, *sourceFakeRepo) {
	t.Helper()
	repo := newSourceFakeRepo()
	uc := usecase.NewSourceTableUseCase(
		repo, sourceFakeFetcher{}, &sourceFakeIDGen{next: "id-test"}, newSilentHandlerLogger(),
	)
	return handler.NewSourceTableHandler(uc), repo
}

func TestSourceTableHandler_AddSourceTable_DetectsHTML(t *testing.T) {
	h, repo := newSourceTableHandler(t)
	id, err := h.AddSourceTable(handler.AddSourceTableRequest{
		URL: "https://example.com/table.html",
	})
	require.NoError(t, err)
	require.Equal(t, "id-test", id)
	require.Equal(t, domain.InputKindHTML, repo.rows["id-test"].InputKind)
	require.Equal(t, "", repo.rows["id-test"].DisplayName)
}

func TestSourceTableHandler_AddSourceTable_DetectsHeaderJSON(t *testing.T) {
	h, repo := newSourceTableHandler(t)
	_, err := h.AddSourceTable(handler.AddSourceTableRequest{
		URL: "https://example.com/header.json",
	})
	require.NoError(t, err)
	require.Equal(t, domain.InputKindHeaderJSON, repo.rows["id-test"].InputKind)
}

func TestSourceTableHandler_AddSourceTable_RejectsEmptyURL(t *testing.T) {
	h, _ := newSourceTableHandler(t)
	_, err := h.AddSourceTable(handler.AddSourceTableRequest{URL: ""})
	require.Error(t, err)
}

func TestSourceTableHandler_ListSourceTables_ReturnsDTOs(t *testing.T) {
	h, repo := newSourceTableHandler(t)
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	repo.rows["x"] = domain.SourceTable{
		ID: "x", InputURL: "u", InputKind: domain.InputKindHTML,
		DisplayName: "Disp", Name: "Name", Symbol: "sym",
		LevelOrder: []string{"0", "1"}, DataURL: "https://x/data.json",
		LastFetchedAt: &now, LastFetchStatus: domain.FetchStatusOK,
	}
	out, err := h.ListSourceTables()
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, "x", out[0].ID)
	require.Equal(t, "u", out[0].InputURL)
	require.Equal(t, "html", out[0].InputKind)
	require.Equal(t, "Disp", out[0].DisplayName)
	require.Equal(t, []string{"0", "1"}, out[0].LevelOrder)
	require.Equal(t, "ok", out[0].LastFetchStatus)
	require.NotEmpty(t, out[0].LastFetchedAt)
}

func TestSourceTableHandler_DeleteSourceTable(t *testing.T) {
	h, repo := newSourceTableHandler(t)
	repo.rows["x"] = domain.SourceTable{ID: "x"}
	require.NoError(t, h.DeleteSourceTable("x"))
	require.NotContains(t, repo.rows, "x")
}

func TestSourceTableHandler_UpdateSourceTableDisplayName(t *testing.T) {
	h, repo := newSourceTableHandler(t)
	repo.rows["x"] = domain.SourceTable{ID: "x", DisplayName: "old"}
	require.NoError(t, h.UpdateSourceTableDisplayName("x", "new"))
	require.Equal(t, "new", repo.rows["x"].DisplayName)
}
