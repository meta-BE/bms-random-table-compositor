package weighter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUniformWeighter_AlwaysReturnsOne(t *testing.T) {
	w := UniformWeighter{}
	for _, a := range []float64{0, 0.25, 0.5, 0.75, 1, 1.5, -0.5} {
		require.Equal(t, 1.0, w.Weight(a))
	}
}
