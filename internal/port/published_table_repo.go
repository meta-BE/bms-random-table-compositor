package port

import (
	"context"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
)

// PublishedTableRepo は published_table の永続化を担う。
type PublishedTableRepo interface {
	List(ctx context.Context) ([]domain.PublishedTable, error)
	Get(ctx context.Context, id string) (domain.PublishedTable, error)
	GetBySlug(ctx context.Context, slug string) (domain.PublishedTable, error)
	// Create は ID を事前採番した PublishedTable を挿入する。slug の UNIQUE 違反は ErrSlugDuplicated で返す。
	Create(ctx context.Context, t domain.PublishedTable) (string, error)
	Update(ctx context.Context, t domain.PublishedTable) error
	Delete(ctx context.Context, id string) error
	// SlugExists は slug が既に使われているかを返す。excludeID を指定すると自分自身は除外（編集時用）。
	SlugExists(ctx context.Context, slug string, excludeID string) (bool, error)
}
