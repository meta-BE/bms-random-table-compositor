package port

import (
	"context"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
)

// Weighter はピック時の重み関数。0 以下を返した譜面は対象外として扱う。
// 最終プレイ日時優先など将来の重み付けはこの差し替え点で実装する。
type Weighter interface {
	Weight(ctx context.Context, ch domain.EnrichedChart, now time.Time) float64
}
