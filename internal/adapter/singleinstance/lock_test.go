package singleinstance

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAcquire_Succeeds_WhenLockFree(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".lock")

	lock, err := Acquire(lockPath)
	require.NoError(t, err)
	defer lock.Release()
	require.NotNil(t, lock)
}

func TestAcquire_Fails_WhenAlreadyHeld(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".lock")

	first, err := Acquire(lockPath)
	require.NoError(t, err)
	defer first.Release()

	_, err = Acquire(lockPath)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrAlreadyRunning)
}

func TestRelease_AllowsReacquire(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".lock")

	first, err := Acquire(lockPath)
	require.NoError(t, err)
	require.NoError(t, first.Release())

	second, err := Acquire(lockPath)
	require.NoError(t, err)
	require.NoError(t, second.Release())
}
