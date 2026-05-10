package weighter

import (
	"context"
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestUniformWeighter_AlwaysReturnsOne(t *testing.T) {
	w := UniformWeighter{}
	now := time.Now()
	ctx := context.Background()

	chart := domain.EnrichedChart{
		SourceChart: domain.SourceChart{MD5: "x", Title: "T"},
	}
	require.Equal(t, 1.0, w.Weight(ctx, chart, now))
}
