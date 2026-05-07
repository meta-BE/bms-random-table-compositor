package httpserver_test

import (
	"context"
	"testing"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/httpserver"
	"github.com/stretchr/testify/require"
)

func TestAdapterServer_StartShutdown(t *testing.T) {
	srv := httpserver.New("127.0.0.1:0", httpserver.Deps{})
	require.NoError(t, srv.Start())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, srv.Shutdown(ctx))
}
