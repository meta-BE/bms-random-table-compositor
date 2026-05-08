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
	"github.com/stretchr/testify/assert"
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

// ---- Refresh テスト ----

func TestSourceTableUseCase_RefreshOne_Success_HTML(t *testing.T) {
	repo := newFakeSourceRepo()
	fetcher := newFakeFetcher()
	repo.rows["id-1"] = domain.SourceTable{
		ID: "id-1", InputURL: "https://x/h.html", InputKind: domain.InputKindHTML,
		LastFetchStatus: domain.FetchStatusNever,
	}
	fetcher.results["https://x/h.html"] = port.FetchedTable{
		Header: domain.BMSTableHeader{Name: "Hello", Symbol: "h"},
		Charts: []domain.SourceChart{
			{Position: 0, MD5: "aaaa", Level: "0", Raw: map[string]any{"md5": "aaaa"}},
		},
		ETag: `"e1"`,
	}
	uc := usecase.NewSourceTableUseCase(repo, fetcher, &fakeIDGen{}, newSilentLogger())
	require.NoError(t, uc.RefreshOne(context.Background(), "id-1"))
	require.Equal(t, 1, fetcher.htmlCalls)
	require.Equal(t, 0, fetcher.headCalls)
	require.Equal(t, "Hello", repo.rows["id-1"].Name)
	require.Equal(t, domain.FetchStatusOK, repo.rows["id-1"].LastFetchStatus)
}

func TestSourceTableUseCase_RefreshOne_Success_HeaderJSON(t *testing.T) {
	repo := newFakeSourceRepo()
	fetcher := newFakeFetcher()
	repo.rows["id-2"] = domain.SourceTable{
		ID: "id-2", InputURL: "https://x/header.json", InputKind: domain.InputKindHeaderJSON,
		LastFetchStatus: domain.FetchStatusNever,
	}
	fetcher.results["https://x/header.json"] = port.FetchedTable{
		Header: domain.BMSTableHeader{Name: "By header", Symbol: "b"},
	}
	uc := usecase.NewSourceTableUseCase(repo, fetcher, &fakeIDGen{}, newSilentLogger())
	require.NoError(t, uc.RefreshOne(context.Background(), "id-2"))
	require.Equal(t, 0, fetcher.htmlCalls)
	require.Equal(t, 1, fetcher.headCalls)
	require.Equal(t, "By header", repo.rows["id-2"].Name)
}

func TestSourceTableUseCase_RefreshOne_FetchError_MarksError(t *testing.T) {
	repo := newFakeSourceRepo()
	fetcher := newFakeFetcher()
	repo.rows["id-3"] = domain.SourceTable{
		ID: "id-3", InputURL: "https://x/h.html", InputKind: domain.InputKindHTML,
		LastFetchStatus: domain.FetchStatusOK, // 前回は成功していた
	}
	fetcher.errs["https://x/h.html"] = errors.New("dns failure")
	uc := usecase.NewSourceTableUseCase(repo, fetcher, &fakeIDGen{}, newSilentLogger())
	require.NoError(t, uc.RefreshOne(context.Background(), "id-3"),
		"取得失敗そのものはエラー扱いにせず、MarkFetchError で記録する")
	require.Equal(t, domain.FetchStatusError, repo.rows["id-3"].LastFetchStatus)
	require.Equal(t, "dns failure", repo.rows["id-3"].LastFetchError)
}

func TestSourceTableUseCase_RefreshOne_NotModified(t *testing.T) {
	repo := newFakeSourceRepo()
	fetcher := newFakeFetcher()
	repo.rows["id-4"] = domain.SourceTable{
		ID: "id-4", InputURL: "https://x/h.html", InputKind: domain.InputKindHTML,
		ETag:            `"prev"`,
		LastFetchStatus: domain.FetchStatusOK,
	}
	fetcher.results["https://x/h.html"] = port.FetchedTable{NotModified: true, ETag: `"prev"`}
	uc := usecase.NewSourceTableUseCase(repo, fetcher, &fakeIDGen{}, newSilentLogger())
	require.NoError(t, uc.RefreshOne(context.Background(), "id-4"))
	saved := repo.saved["id-4"]
	require.True(t, saved.NotModified)
}

func TestSourceTableUseCase_RefreshOne_UnknownIDIsError(t *testing.T) {
	uc := usecase.NewSourceTableUseCase(newFakeSourceRepo(), newFakeFetcher(),
		&fakeIDGen{}, newSilentLogger())
	require.Error(t, uc.RefreshOne(context.Background(), "missing"))
}

func TestSourceTableUseCase_RefreshAll_RunsAllAndContinuesOnError(t *testing.T) {
	repo := newFakeSourceRepo()
	fetcher := newFakeFetcher()
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		repo.rows[id] = domain.SourceTable{
			ID: id, InputURL: "https://x/" + id, InputKind: domain.InputKindHTML,
			LastFetchStatus: domain.FetchStatusNever,
		}
		fetcher.results["https://x/"+id] = port.FetchedTable{
			Header: domain.BMSTableHeader{Name: "n-" + id, Symbol: "s"},
		}
	}
	// 1 件だけわざと失敗させる
	fetcher.results["https://x/c"] = port.FetchedTable{}
	fetcher.errs["https://x/c"] = errors.New("boom")

	uc := usecase.NewSourceTableUseCase(repo, fetcher, &fakeIDGen{}, newSilentLogger())
	require.NoError(t, uc.RefreshAll(context.Background()))

	require.Equal(t, domain.FetchStatusOK, repo.rows["a"].LastFetchStatus)
	require.Equal(t, domain.FetchStatusOK, repo.rows["b"].LastFetchStatus)
	require.Equal(t, domain.FetchStatusError, repo.rows["c"].LastFetchStatus)
	require.Equal(t, domain.FetchStatusOK, repo.rows["d"].LastFetchStatus)
	require.Equal(t, domain.FetchStatusOK, repo.rows["e"].LastFetchStatus)
}

// ---- OnRefreshComplete hook テスト ----

func TestSourceTableUseCase_OnRefreshComplete_FiresOnSuccess(t *testing.T) {
	repo := newFakeSourceRepo()
	fetcher := newFakeFetcher()
	uc := usecase.NewSourceTableUseCase(repo, fetcher,
		&fakeIDGen{ids: []string{"id-1"}}, newSilentLogger())

	const u = "https://example.com/sl/table.html"
	fetcher.results[u] = port.FetchedTable{
		Header: domain.BMSTableHeader{
			Name: "SL", Symbol: "★", LevelOrder: []string{"sl0"},
			DataURL: "https://example.com/sl/data.json",
		},
		Charts: []domain.SourceChart{{Position: 0, MD5: "m1", Level: "sl0", Title: "T"}},
		ETag:   "tag-1",
	}

	id, err := uc.Add(context.Background(), usecase.AddSourceTableInput{URL: u})
	require.NoError(t, err)

	var got []usecase.RefreshCompleteEvent
	uc.OnRefreshComplete(func(e usecase.RefreshCompleteEvent) { got = append(got, e) })

	require.NoError(t, uc.RefreshOne(context.Background(), id))
	require.Len(t, got, 1)
	assert.Equal(t, id, got[0].SourceID)
	assert.Equal(t, domain.FetchStatusOK, got[0].Status)
	assert.Empty(t, got[0].Error)
}

func TestSourceTableUseCase_OnRefreshComplete_FiresOnError(t *testing.T) {
	repo := newFakeSourceRepo()
	fetcher := newFakeFetcher()
	uc := usecase.NewSourceTableUseCase(repo, fetcher,
		&fakeIDGen{ids: []string{"id-1"}}, newSilentLogger())

	const u = "https://example.com/sl/table.html"
	fetcher.errs[u] = errors.New("network error")

	id, err := uc.Add(context.Background(), usecase.AddSourceTableInput{URL: u})
	require.NoError(t, err)

	var got []usecase.RefreshCompleteEvent
	uc.OnRefreshComplete(func(e usecase.RefreshCompleteEvent) { got = append(got, e) })

	_ = uc.RefreshOne(context.Background(), id) // エラーでも通知
	require.Len(t, got, 1)
	assert.Equal(t, domain.FetchStatusError, got[0].Status)
	assert.Contains(t, got[0].Error, "network error")
}
