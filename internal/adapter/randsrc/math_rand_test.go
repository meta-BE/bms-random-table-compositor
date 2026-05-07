package randsrc_test

import (
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/randsrc"
	"github.com/stretchr/testify/require"
)

func TestNewMathRandSource_Deterministic(t *testing.T) {
	a := randsrc.NewMathRandSource(42)
	b := randsrc.NewMathRandSource(42)
	for i := 0; i < 16; i++ {
		require.Equal(t, a.Int63(), b.Int63(), "iter=%d", i)
	}
}

func TestNewMathRandSource_DifferentSeedsDiverge(t *testing.T) {
	a := randsrc.NewMathRandSource(1)
	b := randsrc.NewMathRandSource(2)
	// 連続 8 回のうち 1 回でも違えば OK（同一になる確率は事実上 0）
	diff := false
	for i := 0; i < 8; i++ {
		if a.Int63() != b.Int63() {
			diff = true
			break
		}
	}
	require.True(t, diff)
}
