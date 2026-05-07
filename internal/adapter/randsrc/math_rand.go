// Package randsrc は port.RandSource の math/rand ベース実装を提供する。
package randsrc

import (
	"math/rand"

	"github.com/meta-BE/bms-random-table-compositor/internal/port"
)

type mathRandSource struct {
	src rand.Source
}

// NewMathRandSource は math/rand.NewSource(seed) をラップした port.RandSource を返す。
func NewMathRandSource(seed int64) port.RandSource {
	return &mathRandSource{src: rand.NewSource(seed)}
}

func (m *mathRandSource) Int63() int64    { return m.src.Int63() }
func (m *mathRandSource) Seed(seed int64) { m.src.Seed(seed) }
