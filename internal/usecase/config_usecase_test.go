package usecase_test

import (
	"context"
	"testing"

	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
	"github.com/stretchr/testify/require"
)

// fakeConfigStore は port.ConfigStore のテスト用実装。
type fakeConfigStore struct {
	data map[string]string
}

func newFakeConfigStore() *fakeConfigStore {
	return &fakeConfigStore{data: map[string]string{}}
}

func (f *fakeConfigStore) Get(_ context.Context, key string) (string, bool, error) {
	v, ok := f.data[key]
	return v, ok, nil
}

func (f *fakeConfigStore) Set(_ context.Context, key, value string) error {
	f.data[key] = value
	return nil
}

func TestConfigUseCase_GetServerPort_DefaultsTo50000(t *testing.T) {
	uc := usecase.NewConfigUseCase(newFakeConfigStore())
	port, err := uc.GetServerPort(context.Background())
	require.NoError(t, err)
	require.Equal(t, 50000, port)
}

func TestConfigUseCase_SetThenGetServerPort(t *testing.T) {
	uc := usecase.NewConfigUseCase(newFakeConfigStore())
	require.NoError(t, uc.SetServerPort(context.Background(), 51234))
	port, err := uc.GetServerPort(context.Background())
	require.NoError(t, err)
	require.Equal(t, 51234, port)
}

func TestConfigUseCase_SetServerPort_RejectsOutOfRange(t *testing.T) {
	uc := usecase.NewConfigUseCase(newFakeConfigStore())

	require.Error(t, uc.SetServerPort(context.Background(), 0))
	require.Error(t, uc.SetServerPort(context.Background(), 65536))
	require.Error(t, uc.SetServerPort(context.Background(), -1))
}

func TestConfigUseCase_GetSongdataDBPath_DefaultsToEmpty(t *testing.T) {
	uc := usecase.NewConfigUseCase(newFakeConfigStore())
	p, err := uc.GetSongdataDBPath(context.Background())
	require.NoError(t, err)
	require.Equal(t, "", p)
}

func TestConfigUseCase_SetThenGetSongdataDBPath(t *testing.T) {
	uc := usecase.NewConfigUseCase(newFakeConfigStore())
	require.NoError(t, uc.SetSongdataDBPath(context.Background(), "/tmp/songdata.db"))
	p, err := uc.GetSongdataDBPath(context.Background())
	require.NoError(t, err)
	require.Equal(t, "/tmp/songdata.db", p)
}

func TestConfigUseCase_SetSongdataDBPath_FiresHooks(t *testing.T) {
	store := newFakeConfigStore()
	uc := usecase.NewConfigUseCase(store)
	calls := 0
	uc.AddSongdataPathChangeHook(func() { calls++ })
	uc.AddSongdataPathChangeHook(func() { calls++ })

	require.NoError(t, uc.SetSongdataDBPath(context.Background(), "/path"))
	require.Equal(t, 2, calls)
}

func TestConfigUseCase_GetSetScoreDBPath(t *testing.T) {
	ctx := context.Background()
	store := newFakeConfigStore()
	uc := usecase.NewConfigUseCase(store)

	p, err := uc.GetScoreDBPath(ctx)
	require.NoError(t, err)
	require.Equal(t, "", p)

	require.NoError(t, uc.SetScoreDBPath(ctx, "/abs/score.db"))
	p, err = uc.GetScoreDBPath(ctx)
	require.NoError(t, err)
	require.Equal(t, "/abs/score.db", p)
}

func TestConfigUseCase_AddScorePathChangeHook_FiresOnSet(t *testing.T) {
	ctx := context.Background()
	store := newFakeConfigStore()
	uc := usecase.NewConfigUseCase(store)
	calls := 0
	uc.AddScoreDBPathChangeHook(func() { calls++ })
	require.NoError(t, uc.SetScoreDBPath(ctx, "/a.db"))
	require.NoError(t, uc.SetScoreDBPath(ctx, "/b.db"))
	require.Equal(t, 2, calls)
}
