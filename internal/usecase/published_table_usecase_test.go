package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	"github.com/stretchr/testify/require"
)

// fakePublishedRepo は port.PublishedTableRepo のテスト用実装。
type fakePublishedRepo struct {
	mu    sync.Mutex
	rows  map[string]domain.PublishedTable
	order []string
}

func newFakePublishedRepo() *fakePublishedRepo {
	return &fakePublishedRepo{rows: map[string]domain.PublishedTable{}}
}

func (r *fakePublishedRepo) List(_ context.Context) ([]domain.PublishedTable, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.PublishedTable, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.rows[id])
	}
	return out, nil
}

func (r *fakePublishedRepo) Get(_ context.Context, id string) (domain.PublishedTable, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if v, ok := r.rows[id]; ok {
		return v, nil
	}
	return domain.PublishedTable{}, usecase.ErrPublishedTableNotFound
}

func (r *fakePublishedRepo) GetBySlug(_ context.Context, slug string) (domain.PublishedTable, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, v := range r.rows {
		if v.Slug == slug {
			return v, nil
		}
	}
	return domain.PublishedTable{}, usecase.ErrPublishedTableNotFound
}

func (r *fakePublishedRepo) Create(_ context.Context, t domain.PublishedTable) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, v := range r.rows {
		if v.Slug == t.Slug {
			return "", usecase.ErrSlugDuplicated
		}
	}
	r.rows[t.ID] = t
	r.order = append(r.order, t.ID)
	return t.ID, nil
}

func (r *fakePublishedRepo) Update(_ context.Context, t domain.PublishedTable) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.rows[t.ID]; !ok {
		return usecase.ErrPublishedTableNotFound
	}
	for id, v := range r.rows {
		if id != t.ID && v.Slug == t.Slug {
			return usecase.ErrSlugDuplicated
		}
	}
	r.rows[t.ID] = t
	return nil
}

func (r *fakePublishedRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rows, id)
	for i, v := range r.order {
		if v == id {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
	return nil
}

func (r *fakePublishedRepo) SlugExists(_ context.Context, slug string, excludeID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, v := range r.rows {
		if id != excludeID && v.Slug == slug {
			return true, nil
		}
	}
	return false, nil
}

// pubIDs は PublishedTableUseCase テスト用の決定論的 ID リスト。
// Create 1 回で publishedTable + level 数 + mapping 数の ID を消費するため、
// 余裕を持った件数を生成しておく。
func pubIDs() []string {
	out := make([]string, 0, 200)
	for i := 1; i <= 200; i++ {
		out = append(out, fmt.Sprintf("PUB%020d", i))
	}
	return out
}

func newPublishedUC(t *testing.T, sourceRepo *fakeSourceRepo) (*usecase.PublishedTableUseCase, *fakePublishedRepo) {
	t.Helper()
	pubRepo := newFakePublishedRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	uc := usecase.NewPublishedTableUseCase(pubRepo, sourceRepo, &fakeIDGen{ids: pubIDs()}, logger)
	return uc, pubRepo
}

func seedSource(t *testing.T, repo *fakeSourceRepo, id, name, displayName string) {
	t.Helper()
	_, err := repo.Create(context.Background(), domain.SourceTable{
		ID: id, InputURL: "https://example.com/" + id, InputKind: domain.InputKindHTML,
		Name: name, DisplayName: displayName,
		LastFetchStatus: domain.FetchStatusOK,
	})
	require.NoError(t, err)
}

// seedSourceWithLevels は LevelOrder 付きでソース表を仕込む。CreateFromSourceTable のテスト用。
func seedSourceWithLevels(t *testing.T, repo *fakeSourceRepo, id, name, displayName string, levels []string) {
	t.Helper()
	_, err := repo.Create(context.Background(), domain.SourceTable{
		ID: id, InputURL: "https://example.com/" + id, InputKind: domain.InputKindHTML,
		Name: name, DisplayName: displayName, LevelOrder: levels,
		LastFetchStatus: domain.FetchStatusOK,
	})
	require.NoError(t, err)
}

// singleLevelInput は SourceTableID 1 件 + SourceLevel 1 件のシンプルな Levels を作るヘルパ。
func singleLevelInput(sourceID, name, sourceLevel string) []usecase.PublishedTableLevelInput {
	return []usecase.PublishedTableLevelInput{{
		Name: name,
		Mappings: []usecase.PublishedTableLevelMappingInput{
			{SourceTableID: sourceID, SourceLevel: sourceLevel},
		},
	}}
}

func TestPublishedTableUseCase_Create_Success(t *testing.T) {
	src := newFakeSourceRepo()
	seedSource(t, src, "01JSRC0000000000000000A", "Satellite", "")
	uc, pubRepo := newPublishedUC(t, src)

	id, err := uc.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: "sl-mix", DisplayName: "SL Mix", Symbol: "sl",
		OwnedOnly:   true,
		RefreshMode: domain.RefreshModeDaily,
		Levels: []usecase.PublishedTableLevelInput{{
			Name: "5", PerMappingPick: 5, TotalPick: 5,
			Mappings: []usecase.PublishedTableLevelMappingInput{
				{SourceTableID: "01JSRC0000000000000000A", SourceLevel: "5"},
			},
		}},
	})
	require.NoError(t, err)
	require.NotEmpty(t, id)

	got, err := pubRepo.Get(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, "sl-mix", got.Slug)
	require.True(t, got.OwnedOnly)
	require.Equal(t, domain.RefreshModeDaily, got.Pick.RefreshMode)
	require.Len(t, got.Levels, 1)
	require.Equal(t, "5", got.Levels[0].Name)
	require.Equal(t, 5, got.Levels[0].PerMappingPick)
	require.Equal(t, 5, got.Levels[0].TotalPick)
	require.Len(t, got.Levels[0].Mappings, 1)
}

func TestPublishedTableUseCase_Create_RejectsInvalidSlug(t *testing.T) {
	src := newFakeSourceRepo()
	seedSource(t, src, "01JSRC0000000000000000B", "X", "")
	uc, _ := newPublishedUC(t, src)

	for _, bad := range []string{
		"",            // 空
		"-leading",    // ハイフン始まり
		"UPPER",       // 大文字
		"under_score", // アンダースコア
		"with space",  // スペース
		"あいう",         // マルチバイト
	} {
		_, err := uc.Create(context.Background(), usecase.CreatePublishedTableInput{
			Slug: bad, DisplayName: "X",
			RefreshMode: domain.RefreshModePerRequest,
			Levels:      singleLevelInput("01JSRC0000000000000000B", "5", "5"),
		})
		require.True(t, errors.Is(err, usecase.ErrSlugInvalidFormat),
			"slug=%q expected ErrSlugInvalidFormat, got %v", bad, err)
	}
}

func TestPublishedTableUseCase_Create_RejectsReservedSlug(t *testing.T) {
	src := newFakeSourceRepo()
	seedSource(t, src, "01JSRC0000000000000000C", "X", "")
	uc, _ := newPublishedUC(t, src)

	for _, reserved := range []string{"_admin", "_health", "_metrics", "_refresh", "static", "assets"} {
		_, err := uc.Create(context.Background(), usecase.CreatePublishedTableInput{
			Slug: reserved, DisplayName: "X",
			RefreshMode: domain.RefreshModePerRequest,
			Levels:      singleLevelInput("01JSRC0000000000000000C", "5", "5"),
		})
		require.True(t, errors.Is(err, usecase.ErrSlugReserved) || errors.Is(err, usecase.ErrSlugInvalidFormat),
			"slug=%q expected reserved or invalid, got %v", reserved, err)
	}
}

func TestPublishedTableUseCase_Create_RejectsUnknownSourceTable(t *testing.T) {
	src := newFakeSourceRepo()
	uc, _ := newPublishedUC(t, src)

	// Mappings.SourceTableID が未知の場合 ErrSourceTableNotFound。
	_, err := uc.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: "ok-slug", DisplayName: "X",
		RefreshMode: domain.RefreshModePerRequest,
		Levels:      singleLevelInput("01JSRC0000000000000000Z", "5", "5"),
	})
	require.True(t, errors.Is(err, usecase.ErrSourceTableNotFound))
}

func TestPublishedTableUseCase_Create_RejectsInvalidRefreshMode(t *testing.T) {
	src := newFakeSourceRepo()
	seedSource(t, src, "01JSRC0000000000000000D", "X", "")
	uc, _ := newPublishedUC(t, src)

	_, err := uc.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: "ok-slug", DisplayName: "X",
		RefreshMode: domain.RefreshMode("hourly"),
		Levels:      singleLevelInput("01JSRC0000000000000000D", "5", "5"),
	})
	require.True(t, errors.Is(err, usecase.ErrInvalidRefreshMode))
}

func TestPublishedTableUseCase_Create_RejectsNegativePickPerLevel(t *testing.T) {
	src := newFakeSourceRepo()
	seedSource(t, src, "01JSRC0000000000000000E", "X", "")
	uc, _ := newPublishedUC(t, src)

	_, err := uc.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: "ok-slug", DisplayName: "X",
		RefreshMode: domain.RefreshModePerRequest,
		Levels: []usecase.PublishedTableLevelInput{{
			Name: "5", PerMappingPick: -1,
			Mappings: []usecase.PublishedTableLevelMappingInput{
				{SourceTableID: "01JSRC0000000000000000E", SourceLevel: "5"},
			},
		}},
	})
	require.True(t, errors.Is(err, usecase.ErrInvalidPickCount))
}

func TestPublishedTableUseCase_Create_DuplicateSlugFails(t *testing.T) {
	src := newFakeSourceRepo()
	seedSource(t, src, "01JSRC0000000000000000F", "X", "")
	uc, _ := newPublishedUC(t, src)

	_, err := uc.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: "dup", DisplayName: "A",
		RefreshMode: domain.RefreshModePerRequest,
		Levels:      singleLevelInput("01JSRC0000000000000000F", "5", "5"),
	})
	require.NoError(t, err)
	_, err = uc.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: "dup", DisplayName: "B",
		RefreshMode: domain.RefreshModePerRequest,
		Levels:      singleLevelInput("01JSRC0000000000000000F", "5", "5"),
	})
	require.True(t, errors.Is(err, usecase.ErrSlugDuplicated))
}

func TestPublishedTableUseCase_ValidateSlug(t *testing.T) {
	src := newFakeSourceRepo()
	seedSource(t, src, "01JSRC00000000000000010", "X", "")
	uc, _ := newPublishedUC(t, src)
	id, err := uc.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: "taken", DisplayName: "X",
		RefreshMode: domain.RefreshModePerRequest,
		Levels:      singleLevelInput("01JSRC00000000000000010", "5", "5"),
	})
	require.NoError(t, err)

	require.NoError(t, uc.ValidateSlug(context.Background(), "free-slug", ""))
	require.True(t, errors.Is(uc.ValidateSlug(context.Background(), "Bad_Slug", ""), usecase.ErrSlugInvalidFormat))
	require.True(t, errors.Is(uc.ValidateSlug(context.Background(), "_admin", ""), usecase.ErrSlugReserved))
	require.True(t, errors.Is(uc.ValidateSlug(context.Background(), "taken", ""), usecase.ErrSlugDuplicated))
	// 自分自身を除外すれば OK
	require.NoError(t, uc.ValidateSlug(context.Background(), "taken", id))
}

func TestPublishedTableUseCase_SuggestSlugFromSource(t *testing.T) {
	src := newFakeSourceRepo()
	seedSource(t, src, "01JSRC00000000000000011", "Satellite", "") // Name のみ
	seedSource(t, src, "01JSRC00000000000000012", "X", "発狂難易度表")   // DisplayName 優先 → 全部マルチバイトなのでフォールバック
	seedSource(t, src, "01JSRC00000000000000013", "Stellar Mix β", "")
	uc, _ := newPublishedUC(t, src)

	got, err := uc.SuggestSlugFromSource(context.Background(), "01JSRC00000000000000011")
	require.NoError(t, err)
	require.Equal(t, "satellite", got)

	got, err = uc.SuggestSlugFromSource(context.Background(), "01JSRC00000000000000012")
	require.NoError(t, err)
	// 全部除去された場合は "published" にフォールバック
	require.Equal(t, "published", got)

	got, err = uc.SuggestSlugFromSource(context.Background(), "01JSRC00000000000000013")
	require.NoError(t, err)
	require.Equal(t, "stellar-mix", got)
}

func TestPublishedTableUseCase_SuggestSlugFromSource_AppendsSuffixOnCollision(t *testing.T) {
	src := newFakeSourceRepo()
	seedSource(t, src, "01JSRC00000000000000020", "Satellite", "")
	uc, repo := newPublishedUC(t, src)
	// 既に satellite と satellite-2 が使われているケース
	require.NoError(t, addRow(repo, "PUBA", "satellite"))
	require.NoError(t, addRow(repo, "PUBB", "satellite-2"))

	got, err := uc.SuggestSlugFromSource(context.Background(), "01JSRC00000000000000020")
	require.NoError(t, err)
	require.Equal(t, "satellite-3", got)
}

func addRow(repo *fakePublishedRepo, id, slug string) error {
	_, err := repo.Create(context.Background(), domain.PublishedTable{
		ID: id, Slug: slug, DisplayName: slug,
		Pick: domain.PickConfig{RefreshMode: domain.RefreshModePerRequest},
	})
	return err
}

func TestPublishedTableUseCase_Update_ChecksSlug(t *testing.T) {
	src := newFakeSourceRepo()
	seedSource(t, src, "01JSRC00000000000000030", "X", "")
	uc, _ := newPublishedUC(t, src)
	id1, err := uc.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: "first", DisplayName: "First",
		RefreshMode: domain.RefreshModePerRequest,
		Levels:      singleLevelInput("01JSRC00000000000000030", "5", "5"),
	})
	require.NoError(t, err)
	id2, err := uc.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: "second", DisplayName: "Second",
		RefreshMode: domain.RefreshModePerRequest,
		Levels:      singleLevelInput("01JSRC00000000000000030", "5", "5"),
	})
	require.NoError(t, err)
	_ = id1

	// 自分の slug を別の有効値へ → OK
	require.NoError(t, uc.Update(context.Background(), usecase.UpdatePublishedTableInput{
		ID: id2, Slug: "second-renamed", DisplayName: "Second",
		RefreshMode: domain.RefreshModePerRequest,
		Levels:      singleLevelInput("01JSRC00000000000000030", "5", "5"),
	}))
	// 他人の slug に変更 → 重複
	err = uc.Update(context.Background(), usecase.UpdatePublishedTableInput{
		ID: id2, Slug: "first", DisplayName: "Second",
		RefreshMode: domain.RefreshModePerRequest,
		Levels:      singleLevelInput("01JSRC00000000000000030", "5", "5"),
	})
	require.True(t, errors.Is(err, usecase.ErrSlugDuplicated))
}

func TestPublishedTableUseCase_CreateFromSourceTable_GeneratesLevelsAndMappings(t *testing.T) {
	src := newFakeSourceRepo()
	seedSourceWithLevels(t, src, "01JSRC00000000000000040", "Stella", "", []string{"0", "1", "2"})
	uc, pubRepo := newPublishedUC(t, src)

	id, err := uc.CreateFromSourceTable(context.Background(), "01JSRC00000000000000040", "stella", "Stella Public", "★")
	require.NoError(t, err)
	require.NotEmpty(t, id)

	got, err := pubRepo.Get(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, "stella", got.Slug)
	require.Equal(t, "★", got.Symbol)
	require.Equal(t, domain.RefreshModeManual, got.Pick.RefreshMode)
	require.Len(t, got.Levels, 3)
	require.Equal(t, "0", got.Levels[0].Name)
	require.Equal(t, 0, got.Levels[0].PerMappingPick)
	require.Equal(t, 0, got.Levels[0].TotalPick)
	require.Len(t, got.Levels[0].Mappings, 1)
	require.Equal(t, "01JSRC00000000000000040", got.Levels[0].Mappings[0].SourceTableID)
	require.Equal(t, "0", got.Levels[0].Mappings[0].SourceLevel)
}

func TestPublishedTableUseCase_Create_RejectsDuplicateLevelNames(t *testing.T) {
	src := newFakeSourceRepo()
	seedSource(t, src, "01JSRC00000000000000042", "X", "")
	uc, _ := newPublishedUC(t, src)

	_, err := uc.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: "x", DisplayName: "X", RefreshMode: domain.RefreshModeManual,
		Levels: []usecase.PublishedTableLevelInput{
			{Name: "5", Mappings: []usecase.PublishedTableLevelMappingInput{{SourceTableID: "01JSRC00000000000000042", SourceLevel: "5"}}},
			{Name: "5", Mappings: []usecase.PublishedTableLevelMappingInput{{SourceTableID: "01JSRC00000000000000042", SourceLevel: "6"}}},
		},
	})
	require.True(t, errors.Is(err, usecase.ErrDuplicateLevelName))
}

func TestPublishedTableUseCase_Create_RejectsDuplicateMappingWithinLevel(t *testing.T) {
	src := newFakeSourceRepo()
	seedSource(t, src, "01JSRC00000000000000043", "X", "")
	uc, _ := newPublishedUC(t, src)

	_, err := uc.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: "x", DisplayName: "X", RefreshMode: domain.RefreshModeManual,
		Levels: []usecase.PublishedTableLevelInput{{
			Name: "5",
			Mappings: []usecase.PublishedTableLevelMappingInput{
				{SourceTableID: "01JSRC00000000000000043", SourceLevel: "5"},
				{SourceTableID: "01JSRC00000000000000043", SourceLevel: "5"},
			},
		}},
	})
	require.True(t, errors.Is(err, usecase.ErrDuplicateMapping))
}

func TestPublishedTableUseCase_Create_RejectsUnknownSourceTableInMapping(t *testing.T) {
	src := newFakeSourceRepo()
	uc, _ := newPublishedUC(t, src)

	_, err := uc.Create(context.Background(), usecase.CreatePublishedTableInput{
		Slug: "x", DisplayName: "X", RefreshMode: domain.RefreshModeManual,
		Levels: []usecase.PublishedTableLevelInput{{
			Name:     "5",
			Mappings: []usecase.PublishedTableLevelMappingInput{{SourceTableID: "unknown", SourceLevel: "5"}},
		}},
	})
	require.True(t, errors.Is(err, usecase.ErrSourceTableNotFound))
}
