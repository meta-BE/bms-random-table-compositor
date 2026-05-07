package port

import (
	"context"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
)

// SourceTableRepo は source_table / source_table_chart を永続化する。
type SourceTableRepo interface {
	List(ctx context.Context) ([]domain.SourceTable, error)
	Get(ctx context.Context, id string) (domain.SourceTable, error)
	// Create は SourceTable を新規挿入する。in.ID は事前に採番済みであること。
	Create(ctx context.Context, in domain.SourceTable) (string, error)
	Update(ctx context.Context, t domain.SourceTable) error
	Delete(ctx context.Context, id string) error
	// SaveFetched は取得結果を Tx 内で保存する。
	// NotModified=true の場合は last_fetched_at と updated_at のみ更新し、譜面行は変更しない。
	SaveFetched(ctx context.Context, sourceID string, ft FetchedTable, fetchedAt time.Time) error
	// MarkFetchError は取得失敗を記録する（譜面行は触らない）。
	MarkFetchError(ctx context.Context, sourceID string, fetchErr error, fetchedAt time.Time) error
	// LoadCharts は source_table_chart を position 昇順で返す。
	LoadCharts(ctx context.Context, sourceID string) ([]domain.SourceChart, error)
}
