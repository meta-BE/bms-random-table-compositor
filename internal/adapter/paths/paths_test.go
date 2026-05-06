package paths

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestExecutableDir_ReturnsAbsolutePath(t *testing.T) {
	dir, err := ExecutableDir()
	if err != nil {
		t.Fatalf("ExecutableDir returned error: %v", err)
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("expected absolute path, got %q", dir)
	}
	if strings.TrimSpace(dir) == "" {
		t.Error("expected non-empty directory path")
	}
}

func TestDBPath_IsExecutableDirCompositorDB(t *testing.T) {
	exe, _ := ExecutableDir()
	got, err := DBPath()
	if err != nil {
		t.Fatalf("DBPath returned error: %v", err)
	}
	want := filepath.Join(exe, "compositor.db")
	if got != want {
		t.Errorf("DBPath() = %q, want %q", got, want)
	}
}

func TestLogDir_IsExecutableDirLogs(t *testing.T) {
	exe, _ := ExecutableDir()
	got, err := LogDir()
	if err != nil {
		t.Fatalf("LogDir returned error: %v", err)
	}
	want := filepath.Join(exe, "logs")
	if got != want {
		t.Errorf("LogDir() = %q, want %q", got, want)
	}
}

func TestLockPath_IsExecutableDirDotLock(t *testing.T) {
	exe, _ := ExecutableDir()
	got, err := LockPath()
	if err != nil {
		t.Fatalf("LockPath returned error: %v", err)
	}
	want := filepath.Join(exe, ".lock")
	if got != want {
		t.Errorf("LockPath() = %q, want %q", got, want)
	}
}
