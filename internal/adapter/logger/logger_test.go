package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew_WritesToFile(t *testing.T) {
	dir := t.TempDir()
	logger, closeFn, err := New(Options{
		LogDir:     dir,
		MaxSizeMB:  10,
		MaxBackups: 7,
		MaxAgeDays: 7,
	})
	require.NoError(t, err)

	logger.Info("hello", "key", "value")

	// close で flush される
	require.NoError(t, closeFn())

	matches, err := filepath.Glob(filepath.Join(dir, "*.log"))
	require.NoError(t, err)
	require.NotEmpty(t, matches, "no .log files in %s", dir)

	contents, err := os.ReadFile(matches[0])
	require.NoError(t, err)
	require.True(t, strings.Contains(string(contents), "hello"), "log content: %s", contents)
	require.True(t, strings.Contains(string(contents), "key=value"), "log content: %s", contents)
}

func TestNew_CreatesLogDirIfMissing(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nested", "logs")
	_, closeFn, err := New(Options{
		LogDir:     missing,
		MaxSizeMB:  10,
		MaxBackups: 1,
		MaxAgeDays: 1,
	})
	require.NoError(t, err)
	defer closeFn()

	info, err := os.Stat(missing)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}
