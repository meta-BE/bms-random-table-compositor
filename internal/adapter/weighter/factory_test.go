package weighter

import (
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestFactory_OffReturnsUniform(t *testing.T) {
	f := Factory{}
	w := f.For(domain.PickConfig{WeightMode: domain.WeightModeOff})
	_, ok := w.(UniformWeighter)
	require.True(t, ok, "WeightMode=off should yield UniformWeighter")
}

func TestFactory_SortReturnsUniform(t *testing.T) {
	f := Factory{}
	w := f.For(domain.PickConfig{WeightMode: domain.WeightModeSort})
	_, ok := w.(UniformWeighter)
	require.True(t, ok, "WeightMode=sort uses別経路だが Factory 返却は Uniform で安全側")
}

func TestFactory_ProbabilityReturnsLastPlayed(t *testing.T) {
	f := Factory{}
	w := f.For(domain.PickConfig{WeightMode: domain.WeightModeProbability, WeightParamX: 10})
	lp, ok := w.(LastPlayedWeighter)
	require.True(t, ok)
	require.Equal(t, 10.0, lp.X)
}

func TestFactory_ProbabilityClampsBelowOne(t *testing.T) {
	f := Factory{}
	w := f.For(domain.PickConfig{WeightMode: domain.WeightModeProbability, WeightParamX: 0})
	lp, ok := w.(LastPlayedWeighter)
	require.True(t, ok)
	require.Equal(t, 1.0, lp.X, "X<1 は 1 に clamp")
}
