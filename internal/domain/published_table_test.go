package domain_test

import (
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestRefreshMode_Values(t *testing.T) {
	require.Equal(t, domain.RefreshMode("per_request"), domain.RefreshModePerRequest)
	require.Equal(t, domain.RefreshMode("daily"), domain.RefreshModeDaily)
	require.Equal(t, domain.RefreshMode("manual"), domain.RefreshModeManual)
}

func TestServerState_Values(t *testing.T) {
	require.Equal(t, domain.ServerState("stopped"), domain.ServerStateStopped)
	require.Equal(t, domain.ServerState("running"), domain.ServerStateRunning)
	require.Equal(t, domain.ServerState("error"), domain.ServerStateError)
}
