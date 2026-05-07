package clock_test

import (
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/clock"
	"github.com/stretchr/testify/require"
)

func TestSystemClock_Now_IsRecent(t *testing.T) {
	c := clock.System{}
	before := time.Now()
	got := c.Now()
	after := time.Now()
	require.False(t, got.Before(before))
	require.False(t, got.After(after))
}
