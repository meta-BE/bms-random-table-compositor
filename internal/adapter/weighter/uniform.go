// Package weighter は port.Weighter / port.WeighterFactory の実装群。
package weighter

// UniformWeighter は全譜面に等しく 1 を返す。WeightMode=off で使用。
// WeightMode=sort 経路では Weighter 自体を使わないが、Factory の安全側として返却される。
type UniformWeighter struct{}

func (UniformWeighter) Weight(_ float64) float64 { return 1 }
