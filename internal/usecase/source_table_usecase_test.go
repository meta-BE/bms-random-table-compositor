package usecase_test

import (
	"context"
	"errors"
	"io/ioutil"
	"log/slog"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	"github.com/stretchr/testify/require"
)

// fakeSourceRepo は port.SourceTableRepo のテスト用実装。
type fakeSourceRepo struct {
	mu     sync.Mutex
	rows   map[string]domain.SourceTable
	charts map[string][]domain.SourceChart
	saved  map[string]port.FetchedTable
	errs   map[string]string
}

func newFakeSourceRepo() *fakeSourceRepo {
	return &fakeSourceRepo{
		rows: map[string]domain.SourceTable{}, charts: map[string][]domain.SourceChart{},
		saved: map[string]port.FetchedTable{}, errs: map[string]string{},
	}
}

func (r *fakeSourceRepo) List(_ context.Context) ([]domain.SourceTable, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.SourceTable, 0, len(r.rows))
	for _, v := range r.rows {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *fakeSourceRepo) Get(_ context.Context, id string) (domain.SourceTable, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.rows[id]
	if !ok {
		return domain.SourceTable{}, errors.New("not found")
	}
	return v, nil
}

func (r *fakeSourceRepo) Create(_ context.Context, in domain.SourceTable) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows[in.ID] = in
	return in.ID, nil
}

func (r *fakeSourceRepo) Update(_ context.Context, t domain.SourceTable) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.rows[t.ID]; !ok {
		return errors.New("not found")
	}
	r.rows[t.ID] = t
	return nil
}

func (r *fakeSourceRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rows, id)
	return nil
}

func (r *fakeSourceRepo) SaveFetched(_ context.Context, id string, ft port.FetchedTable, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.saved[id] = ft
	row := r.rows[id]
	if !ft.NotModified {
		row.Name = ft.Header.Name
		row.Symbol = ft.Header.Symbol
		row.LevelOrder = ft.Header.LevelOrder
		row.DataURL = ft.Header.DataURL
		row.ETag = ft.ETag
		row.LastFetchError = ""
		r.charts[id] = ft.Charts
	}
	row.LastFetchedAt = &at
	row.LastFetchStatus = domain.FetchStatusOK
	r.rows[id] = row
	return nil
}

func (r *fakeSourceRepo) MarkFetchError(_ context.Context, id string, e error, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	row := r.rows[id]
	row.LastFetchedAt = &at
	row.LastFetchStatus = domain.FetchStatusError
	row.LastFetchError = e.Error()
	r.rows[id] = row
	r.errs[id] = e.Error()
	return nil
}

func (r *fakeSourceRepo) LoadCharts(_ context.Context, id string) ([]domain.SourceChart, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.charts[id], nil
}

// fakeFetcher は port.SourceTableFetcher のテスト用実装。
type fakeFetcher struct {
	mu        sync.Mutex
	htmlCalls int
	headCalls int
	results   map[string]port.FetchedTable
	errs      map[string]error
}

func newFakeFetcher() *fakeFetcher {
	return &fakeFetcher{results: map[string]port.FetchedTable{}, errs: map[string]error{}}
}

func (f *fakeFetcher) FetchByHTML(_ context.Context, u string, _ string) (port.FetchedTable, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.htmlCalls++
	if e, ok := f.errs[u]; ok {
		return port.FetchedTable{}, e
	}
	return f.results[u], nil
}

func (f *fakeFetcher) FetchByHeader(_ context.Context, u string, _ string) (port.FetchedTable, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.headCalls++
	if e, ok := f.errs[u]; ok {
		return port.FetchedTable{}, e
	}
	return f.results[u], nil
}

// fakeIDGen は決定論的に ID を返す。
type fakeIDGen struct {
	ids []string
	i   int
}

func (g *fakeIDGen) New() string {
	v := g.ids[g.i]
	g.i++
	return v
}

func newSilentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(ioutil.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// ---- CRUD テスト ----

func TestSourceTableUseCase_Add_RejectsEmptyURL(t *testing.T) {
	uc := usecase.NewSourceTableUseCase(newFakeSourceRepo(), newFakeFetcher(),
		&fakeIDGen{ids: []string{"id-1"}}, newSilentLogger())
	_, err := uc.Add(context.Background(), usecase.AddSourceTableInput{URL: ""})
	require.Error(t, err)
}

func TestSourceTableUseCase_Add_RejectsMalformedURL(t *testing.T) {
	uc := usecase.NewSourceTableUseCase(newFakeSourceRepo(), newFakeFetcher(),
		&fakeIDGen{ids: []string{"id-1"}}, newSilentLogger())
	_, err := uc.Add(context.Background(), usecase.AddSourceTableInput{URL: "not-a-url"})
	require.Error(t, err)
}

func TestSourceTableUseCase_Add_DetectsHTMLByDefault(t *testing.T) {
	repo := newFakeSourceRepo()
	uc := usecase.NewSourceTableUseCase(repo, newFakeFetcher(),
		&fakeIDGen{ids: []string{"id-X"}}, newSilentLogger())
	id, err := uc.Add(context.Background(), usecase.AddSourceTableInput{
		URL: "https://example.com/sl/table.html",
	})
	require.NoError(t, err)
	require.Equal(t, "id-X", id)
	require.Equal(t, domain.InputKindHTML, repo.rows[id].InputKind)
	require.Equal(t, "", repo.rows[id].DisplayName,
		"DisplayName は初期値 空。取得後に Name で UI 側がフォールバック表示する")
	require.Equal(t, domain.FetchStatusNever, repo.rows[id].LastFetchStatus)
}

func TestSourceTableUseCase_Add_DetectsHeaderJSONByExtension(t *testing.T) {
	repo := newFakeSourceRepo()
	uc := usecase.NewSourceTableUseCase(repo, newFakeFetcher(),
		&fakeIDGen{ids: []string{"id-Y"}}, newSilentLogger())
	id, err := uc.Add(context.Background(), usecase.AddSourceTableInput{
		URL: "https://example.com/sl/header.json",
	})
	require.NoError(t, err)
	require.Equal(t, domain.InputKindHeaderJSON, repo.rows[id].InputKind)
}

func TestSourceTableUseCase_Add_JSONExtCaseInsensitive(t *testing.T) {
	repo := newFakeSourceRepo()
	uc := usecase.NewSourceTableUseCase(repo, newFakeFetcher(),
		&fakeIDGen{ids: []string{"id-Z"}}, newSilentLogger())
	id, err := uc.Add(context.Background(), usecase.AddSourceTableInput{
		URL: "https://example.com/sl/HEADER.JSON",
	})
	require.NoError(t, err)
	require.Equal(t, domain.InputKindHeaderJSON, repo.rows[id].InputKind)
}

func TestSourceTableUseCase_Add_QueryStringIgnoredForKind(t *testing.T) {
	repo := newFakeSourceRepo()
	uc := usecase.NewSourceTableUseCase(repo, newFakeFetcher(),
		&fakeIDGen{ids: []string{"id-Q"}}, newSilentLogger())
	id, err := uc.Add(context.Background(), usecase.AddSourceTableInput{
		URL: "https://example.com/sl/header.json?cb=42",
	})
	require.NoError(t, err)
	require.Equal(t, domain.InputKindHeaderJSON, repo.rows[id].InputKind,
		"クエリ文字列は path 末尾の判定に影響しない")
}

func TestSourceTableUseCase_List_PassThrough(t *testing.T) {
	repo := newFakeSourceRepo()
	repo.rows["a"] = domain.SourceTable{ID: "a"}
	repo.rows["b"] = domain.SourceTable{ID: "b"}
	uc := usecase.NewSourceTableUseCase(repo, newFakeFetcher(), &fakeIDGen{}, newSilentLogger())
	out, err := uc.List(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 2)
}

func TestSourceTableUseCase_Remove_PassThrough(t *testing.T) {
	repo := newFakeSourceRepo()
	repo.rows["a"] = domain.SourceTable{ID: "a"}
	uc := usecase.NewSourceTableUseCase(repo, newFakeFetcher(), &fakeIDGen{}, newSilentLogger())
	require.NoError(t, uc.Remove(context.Background(), "a"))
	require.NotContains(t, repo.rows, "a")
}

func TestSourceTableUseCase_UpdateDisplayName_OverwritesField(t *testing.T) {
	repo := newFakeSourceRepo()
	repo.rows["a"] = domain.SourceTable{ID: "a", DisplayName: "old"}
	uc := usecase.NewSourceTableUseCase(repo, newFakeFetcher(), &fakeIDGen{}, newSilentLogger())
	require.NoError(t, uc.UpdateDisplayName(context.Background(), "a", "new"))
	require.Equal(t, "new", repo.rows["a"].DisplayName)
}
