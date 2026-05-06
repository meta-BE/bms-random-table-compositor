package persistence

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func setupConfigStore(t *testing.T) (*ConfigStoreSQL, func()) {
	t.Helper()
	dir := t.TempDir()
	db, err := OpenDB(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	require.NoError(t, RunMigrations(db))
	return NewConfigStoreSQL(db), func() { db.Close() }
}

func TestConfigStoreSQL_Get_NotFound(t *testing.T) {
	store, cleanup := setupConfigStore(t)
	defer cleanup()

	_, found, err := store.Get(context.Background(), "missing_key")
	require.NoError(t, err)
	require.False(t, found)
}

func TestConfigStoreSQL_SetThenGet_RoundTrip(t *testing.T) {
	store, cleanup := setupConfigStore(t)
	defer cleanup()

	require.NoError(t, store.Set(context.Background(), "server_port", "50000"))

	v, found, err := store.Get(context.Background(), "server_port")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "50000", v)
}

func TestConfigStoreSQL_Set_Overwrites(t *testing.T) {
	store, cleanup := setupConfigStore(t)
	defer cleanup()
	ctx := context.Background()

	require.NoError(t, store.Set(ctx, "server_port", "50000"))
	require.NoError(t, store.Set(ctx, "server_port", "51234"))

	v, _, err := store.Get(ctx, "server_port")
	require.NoError(t, err)
	require.Equal(t, "51234", v)
}
