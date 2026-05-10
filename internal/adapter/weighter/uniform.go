// Package weighter は port.Weighter の実装群。
package weighter

import (
	"context"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
)

// UniformWeighter は全譜面に等しく 1 を返す。MVP デフォルト。
type UniformWeighter struct{}

func (UniformWeighter) Weight(_ context.Context, _ domain.EnrichedChart, _ time.Time) float64 {
	return 1
}
