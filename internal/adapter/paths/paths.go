// Package paths は実行ファイル隣の各種パス（DB、ログディレクトリ）を算出するヘルパーを提供する。
package paths

import (
	"os"
	"path/filepath"
)

const (
	dbFilename = "compositor.db"
	logDirname = "logs"
)

// ExecutableDir は実行ファイルが置かれているディレクトリの絶対パスを返す。
func ExecutableDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		resolved = exe
	}
	return filepath.Dir(resolved), nil
}

// DBPath は compositor.db の絶対パスを返す。
func DBPath() (string, error) {
	dir, err := ExecutableDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, dbFilename), nil
}

// LogDir はログディレクトリの絶対パスを返す。
func LogDir() (string, error) {
	dir, err := ExecutableDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, logDirname), nil
}
