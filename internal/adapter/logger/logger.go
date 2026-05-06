// Package logger は slog + lumberjack による日次ローテーションログを提供する。
package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

// Options は Logger の設定を表す。
type Options struct {
	// LogDir はログファイルを格納するディレクトリ。なければ作成する。
	LogDir string
	// MaxSizeMB は1ファイルの最大サイズ。超過時に新ファイルへローテ。
	MaxSizeMB int
	// MaxBackups はローテ後に保持する旧ファイル数。
	MaxBackups int
	// MaxAgeDays は旧ファイルを保持する最大日数。
	MaxAgeDays int
}

// CloseFunc は Logger 関連リソースを開放するクロージャ。
type CloseFunc func() error

// New は Options に基づき *slog.Logger を返す。
// ログ出力は LogDir/<YYYY-MM-DD>.log に書かれ、stderr にもミラーされる。
func New(opts Options) (*slog.Logger, CloseFunc, error) {
	if err := os.MkdirAll(opts.LogDir, 0o755); err != nil {
		return nil, noopClose, fmt.Errorf("mkdir log dir: %w", err)
	}
	filename := filepath.Join(opts.LogDir, time.Now().Format("2006-01-02")+".log")
	rotator := &lumberjack.Logger{
		Filename:   filename,
		MaxSize:    opts.MaxSizeMB,
		MaxBackups: opts.MaxBackups,
		MaxAge:     opts.MaxAgeDays,
		Compress:   false,
	}

	writer := io.MultiWriter(rotator, os.Stderr)
	handler := slog.NewTextHandler(writer, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

	return logger, rotator.Close, nil
}

func noopClose() error { return nil }
