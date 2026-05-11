package handler_test

import (
	"context"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/app/handler"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	"github.com/stretchr/testify/require"
)

type fakeStore struct {
	data map[string]string
}

func (f *fakeStore) Get(_ context.Context, k string) (string, bool, error) {
	v, ok := f.data[k]
	return v, ok, nil
}
func (f *fakeStore) Set(_ context.Context, k, v string) error {
	f.data[k] = v
	return nil
}

func newHandler() *handler.ConfigHandler {
	uc := usecase.NewConfigUseCase(&fakeStore{data: map[string]string{}})
	return handler.NewConfigHandler(uc)
}

func TestConfigHandler_GetServerConfig_DefaultValues(t *testing.T) {
	h := newHandler()
	cfg, err := h.GetServerConfig()
	require.NoError(t, err)
	require.Equal(t, 50000, cfg.Port)
	require.Equal(t, "", cfg.SongdataDBPath)
}

func TestConfigHandler_SetServerPort_Persists(t *testing.T) {
	h := newHandler()
	require.NoError(t, h.SetServerPort(51234))
	cfg, _ := h.GetServerConfig()
	require.Equal(t, 51234, cfg.Port)
}

func TestConfigHandler_SetServerPort_RejectsOutOfRange(t *testing.T) {
	h := newHandler()
	require.Error(t, h.SetServerPort(0))
	require.Error(t, h.SetServerPort(70000))
}

func TestConfigHandler_SetSongdataDBPath_Persists(t *testing.T) {
	h := newHandler()
	require.NoError(t, h.SetSongdataDBPath("/tmp/songdata.db"))
	cfg, _ := h.GetServerConfig()
	require.Equal(t, "/tmp/songdata.db", cfg.SongdataDBPath)
}

func TestConfigHandler_PickSongdataDB_NoContext(t *testing.T) {
	t.Parallel()
	uc := usecase.NewConfigUseCase(&fakeStore{data: map[string]string{}})
	h := handler.NewConfigHandler(uc)
	// SetContext 前は ctx が context.Background() 固定。runtime API は呼ばない契約とする。
	got, err := h.PickSongdataDB()
	require.NoError(t, err)
	require.Equal(t, "", got)
}

func TestConfigHandler_GetServerConfig_IncludesScoreDBPath(t *testing.T) {
	h := newHandler()
	cfg, err := h.GetServerConfig()
	require.NoError(t, err)
	require.Equal(t, "", cfg.ScoreDBPath)
}

func TestConfigHandler_SetScoreDBPath_Persists(t *testing.T) {
	h := newHandler()
	require.NoError(t, h.SetScoreDBPath("/tmp/score.db"))
	cfg, err := h.GetServerConfig()
	require.NoError(t, err)
	require.Equal(t, "/tmp/score.db", cfg.ScoreDBPath)
}

func TestConfigHandler_PickScoreDB_NoContext(t *testing.T) {
	t.Parallel()
	uc := usecase.NewConfigUseCase(&fakeStore{data: map[string]string{}})
	h := handler.NewConfigHandler(uc)
	// SetContext 前は ctx が context.Background() 固定。runtime API は呼ばない契約とする。
	got, err := h.PickScoreDB()
	require.NoError(t, err)
	require.Equal(t, "", got, "SetContext 前は空文字")
}
