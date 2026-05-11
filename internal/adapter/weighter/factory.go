package weighter

import (
	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
)

// Factory は port.WeighterFactory の実装。
// domain.PickConfig から適切な Weighter を選ぶ。
type Factory struct{}

func (Factory) For(cfg domain.PickConfig) port.Weighter {
	switch cfg.WeightMode {
	case domain.WeightModeProbability:
		x := float64(cfg.WeightParamX)
		if x < 1 {
			x = 1
		}
		return LastPlayedWeighter{X: x}
	default:
		return UniformWeighter{}
	}
}
