package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
)

// SourceTableUseCase はソース表の CRUD と取得（refresh）のビジネスロジックを束ねる。
type SourceTableUseCase struct {
	repo    port.SourceTableRepo
	fetcher port.SourceTableFetcher
	idGen   port.IDGenerator
	log     *slog.Logger
}

// NewSourceTableUseCase は新しい SourceTableUseCase を作る。
func NewSourceTableUseCase(
	repo port.SourceTableRepo,
	fetcher port.SourceTableFetcher,
	idGen port.IDGenerator,
	log *slog.Logger,
) *SourceTableUseCase {
	return &SourceTableUseCase{repo: repo, fetcher: fetcher, idGen: idGen, log: log}
}

// AddSourceTableInput は Add が受け取る入力。InputKind と DisplayName は
// それぞれ URL からの自動判別 / 取得後の Name フォールバックで埋めるため、
// ユーザーには入力させない。
type AddSourceTableInput struct {
	URL string
}

// Add は SourceTable を新規登録する。InputKind は URL の path 拡張子から判別する
// （`.json` で終われば HeaderJSON、それ以外は HTML）。実取得は呼び出し側が
// RefreshOne で行うため、DisplayName / Name / Symbol 等の表メタは初期値（空）
// で挿入される。フロントエンドは取得後に `displayName || name` の優先で表示する。
func (u *SourceTableUseCase) Add(ctx context.Context, in AddSourceTableInput) (string, error) {
	if in.URL == "" {
		return "", errors.New("URL は必須です")
	}
	kind, err := inferInputKind(in.URL)
	if err != nil {
		return "", err
	}
	id := u.idGen.New()
	st := domain.SourceTable{
		ID: id, InputURL: in.URL, InputKind: kind,
		LastFetchStatus: domain.FetchStatusNever,
	}
	return u.repo.Create(ctx, st)
}

// inferInputKind は URL を解析し、path 末尾が ".json"（大文字小文字無視）の場合は
// HeaderJSON、それ以外は HTML として扱う。GAS のような拡張子なしで JSON を返す URL
// は HTML 判定されてしまうが、header.json を返す GAS は実用上ほぼ存在しないため
// Phase 1 ではこの単純ルールで割り切る（必要に応じて将来 Content-Type ベースの
// フォールバックを追加）。
func inferInputKind(rawURL string) (domain.InputKind, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("URL のパースに失敗 %q: %w", rawURL, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("URL の形式が不正です: %s", rawURL)
	}
	if strings.HasSuffix(strings.ToLower(u.Path), ".json") {
		return domain.InputKindHeaderJSON, nil
	}
	return domain.InputKindHTML, nil
}

// List はすべての SourceTable を返す。
func (u *SourceTableUseCase) List(ctx context.Context) ([]domain.SourceTable, error) {
	return u.repo.List(ctx)
}

// Get は指定 ID の SourceTable を返す。
func (u *SourceTableUseCase) Get(ctx context.Context, id string) (domain.SourceTable, error) {
	return u.repo.Get(ctx, id)
}

// Remove は SourceTable を削除する。譜面行は外部キー ON DELETE CASCADE で連動削除される。
func (u *SourceTableUseCase) Remove(ctx context.Context, id string) error {
	return u.repo.Delete(ctx, id)
}

// UpdateDisplayName は表示名のみ書き換える（他フィールドは fetcher が更新する責務）。
func (u *SourceTableUseCase) UpdateDisplayName(ctx context.Context, id string, displayName string) error {
	st, err := u.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	st.DisplayName = displayName
	return u.repo.Update(ctx, st)
}

// RefreshOne は単一 SourceTable を取得・保存する。
// 取得失敗自体はエラーとして返さず、Repo.MarkFetchError で記録して nil を返す
// （RefreshAll の途中で goroutine を止めないため）。
// Repo の永続化失敗は通常エラーで返す。
func (u *SourceTableUseCase) RefreshOne(ctx context.Context, id string) error {
	st, err := u.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	now := time.Now()

	var (
		fetched  port.FetchedTable
		fetchErr error
	)
	switch st.InputKind {
	case domain.InputKindHTML:
		fetched, fetchErr = u.fetcher.FetchByHTML(ctx, st.InputURL, st.ETag)
	case domain.InputKindHeaderJSON:
		fetched, fetchErr = u.fetcher.FetchByHeader(ctx, st.InputURL, st.ETag)
	default:
		fetchErr = fmt.Errorf("不正な input_kind %q", st.InputKind)
	}

	if fetchErr != nil {
		u.log.Warn("source table refresh failed",
			"id", id, "url", st.InputURL, "err", fetchErr)
		if mErr := u.repo.MarkFetchError(ctx, id, fetchErr, now); mErr != nil {
			return fmt.Errorf("mark fetch error: %w", mErr)
		}
		return nil
	}

	if err := u.repo.SaveFetched(ctx, id, fetched, now); err != nil {
		u.log.Error("source table save failed", "id", id, "err", err)
		return fmt.Errorf("save fetched: %w", err)
	}
	u.log.Info("source table refreshed",
		"id", id, "name", fetched.Header.Name,
		"charts", len(fetched.Charts), "notModified", fetched.NotModified)
	return nil
}

// RefreshAll は登録済み全 SourceTable を並列度 4 で RefreshOne する。
// 個別失敗は Repo に記録され、RefreshAll 自体はエラーを返さない（List 失敗のみ伝播）。
func (u *SourceTableUseCase) RefreshAll(ctx context.Context) error {
	list, err := u.repo.List(ctx)
	if err != nil {
		return fmt.Errorf("list source tables: %w", err)
	}
	const concurrency = 4
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for _, st := range list {
		id := st.ID
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if err := u.RefreshOne(ctx, id); err != nil {
				u.log.Warn("refresh all: one failed", "id", id, "err", err)
			}
		}()
	}
	wg.Wait()
	return nil
}
