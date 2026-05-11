package weighter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLastPlayedWeighter_LinearInterpolation(t *testing.T) {
	cases := []struct {
		x        float64
		a        float64
		expected float64
	}{
		{x: 1, a: 0, expected: 1},
		{x: 1, a: 1, expected: 1},
		{x: 2, a: 0, expected: 1},
		{x: 2, a: 0.5, expected: 1.5},
		{x: 2, a: 1, expected: 2},
		{x: 10, a: 0, expected: 1},
		{x: 10, a: 0.5, expected: 5.5},
		{x: 10, a: 1, expected: 10},
		{x: 100, a: 0, expected: 1},
		{x: 100, a: 1, expected: 100},
	}
	for _, c := range cases {
		w := LastPlayedWeighter{X: c.x}
		require.InDelta(t, c.expected, w.Weight(c.a), 1e-9, "X=%v a=%v", c.x, c.a)
	}
}
