package usecase_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	"github.com/stretchr/testify/require"
)

type fakeHTTPServer struct {
	mu       sync.Mutex
	addr     string
	startErr error
	stopErr  error
	started  bool
	stopped  bool
	startCnt int
	stopCnt  int
}

func (s *fakeHTTPServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startCnt++
	if s.startErr != nil {
		return s.startErr
	}
	s.started = true
	return nil
}

func (s *fakeHTTPServer) Shutdown(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopCnt++
	s.stopped = true
	s.started = false
	return s.stopErr
}

func (s *fakeHTTPServer) Addr() string { return s.addr }

func newServerUC(t *testing.T, store *fakeConfigStore, factory func(addr string) usecase.HTTPServer) *usecase.ServerUseCase {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return usecase.NewServerUseCase(store, factory, logger)
}

func TestServerUseCase_Start_TransitionsToRunning(t *testing.T) {
	store := newFakeConfigStore()
	require.NoError(t, store.Set(context.Background(), "server_port", "50000"))
	srv := &fakeHTTPServer{addr: ":50000"}
	uc := newServerUC(t, store, func(addr string) usecase.HTTPServer { return srv })

	require.NoError(t, uc.Start(context.Background()))
	require.Equal(t, domain.ServerStateRunning, uc.Status().State)
	require.Equal(t, 50000, uc.Status().Port)
	require.Equal(t, 1, srv.startCnt)
}

func TestServerUseCase_Start_FailureSetsErrorState(t *testing.T) {
	store := newFakeConfigStore()
	require.NoError(t, store.Set(context.Background(), "server_port", "50000"))
	srv := &fakeHTTPServer{addr: ":50000", startErr: errors.New("EADDRINUSE")}
	uc := newServerUC(t, store, func(addr string) usecase.HTTPServer { return srv })

	err := uc.Start(context.Background())
	require.Error(t, err)
	st := uc.Status()
	require.Equal(t, domain.ServerStateError, st.State)
	require.Contains(t, st.LastError, "EADDRINUSE")
}

func TestServerUseCase_Start_FailsIfAlreadyRunning(t *testing.T) {
	store := newFakeConfigStore()
	require.NoError(t, store.Set(context.Background(), "server_port", "50000"))
	srv := &fakeHTTPServer{addr: ":50000"}
	uc := newServerUC(t, store, func(addr string) usecase.HTTPServer { return srv })

	require.NoError(t, uc.Start(context.Background()))
	err := uc.Start(context.Background())
	require.True(t, errors.Is(err, usecase.ErrServerAlreadyRunning))
}

func TestServerUseCase_Stop_AfterStart(t *testing.T) {
	store := newFakeConfigStore()
	require.NoError(t, store.Set(context.Background(), "server_port", "50000"))
	srv := &fakeHTTPServer{addr: ":50000"}
	uc := newServerUC(t, store, func(addr string) usecase.HTTPServer { return srv })

	require.NoError(t, uc.Start(context.Background()))
	require.NoError(t, uc.Stop(context.Background()))
	require.Equal(t, domain.ServerStateStopped, uc.Status().State)
	require.Equal(t, 1, srv.stopCnt)
}

func TestServerUseCase_Stop_FailsIfNotRunning(t *testing.T) {
	store := newFakeConfigStore()
	uc := newServerUC(t, store, func(addr string) usecase.HTTPServer { return &fakeHTTPServer{} })
	err := uc.Stop(context.Background())
	require.True(t, errors.Is(err, usecase.ErrServerNotRunning))
}

func TestServerUseCase_Restart(t *testing.T) {
	store := newFakeConfigStore()
	require.NoError(t, store.Set(context.Background(), "server_port", "50000"))

	calls := 0
	uc := newServerUC(t, store, func(addr string) usecase.HTTPServer {
		calls++
		return &fakeHTTPServer{addr: addr}
	})
	require.NoError(t, uc.Start(context.Background()))
	require.NoError(t, uc.Restart(context.Background()))
	require.Equal(t, domain.ServerStateRunning, uc.Status().State)
	require.Equal(t, 2, calls, "factory が再呼出しされたはず")
}

func TestServerUseCase_OnStatusChange_NotifiesListeners(t *testing.T) {
	store := newFakeConfigStore()
	require.NoError(t, store.Set(context.Background(), "server_port", "50000"))
	uc := newServerUC(t, store, func(addr string) usecase.HTTPServer { return &fakeHTTPServer{addr: addr} })

	var mu sync.Mutex
	statuses := []domain.ServerStatus{}
	uc.OnStatusChange(func(s domain.ServerStatus) {
		mu.Lock()
		defer mu.Unlock()
		statuses = append(statuses, s)
	})

	require.NoError(t, uc.Start(context.Background()))
	require.NoError(t, uc.Stop(context.Background()))

	mu.Lock()
	defer mu.Unlock()
	require.GreaterOrEqual(t, len(statuses), 2)
	require.Equal(t, domain.ServerStateRunning, statuses[0].State)
	require.Equal(t, domain.ServerStateStopped, statuses[len(statuses)-1].State)
}
