// Package app は Wails Bind ターゲットとなるハンドラ群と、サービス起動の配線を提供する。
package app

import (
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/logger"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/paths"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/persistence"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/singleinstance"
	"github.com/meta-BE/bms-random-table-compositor/internal/app/handler"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

// Services はアプリ全体で共有する依存を保持する。
type Services struct {
	DB            *sql.DB
	Logger        *slog.Logger
	LoggerClose   logger.CloseFunc
	Lock          singleinstance.Lock
	ConfigHandler *handler.ConfigHandler
}

// Bootstrap は Services を構築する（DB接続・マイグレーション・ロック取得・ロガー初期化）。
// 失敗時は途中で取得済みのリソースを開放してエラーを返す。
func Bootstrap() (*Services, error) {
	// 1. ログディレクトリとロガー
	logDir, err := paths.LogDir()
	if err != nil {
		return nil, fmt.Errorf("log dir: %w", err)
	}
	lg, closeLog, err := logger.New(logger.Options{
		LogDir:     logDir,
		MaxSizeMB:  50,
		MaxBackups: 7,
		MaxAgeDays: 7,
	})
	if err != nil {
		return nil, fmt.Errorf("logger init: %w", err)
	}

	// 2. シングルインスタンスロック
	lockPath, err := paths.LockPath()
	if err != nil {
		_ = closeLog()
		return nil, fmt.Errorf("lock path: %w", err)
	}
	lock, err := singleinstance.Acquire(lockPath)
	if err != nil {
		_ = closeLog()
		return nil, err // ErrAlreadyRunning も含む
	}

	// 3. DB と マイグレーション
	dbPath, err := paths.DBPath()
	if err != nil {
		_ = lock.Release()
		_ = closeLog()
		return nil, fmt.Errorf("db path: %w", err)
	}
	db, err := persistence.OpenDB(dbPath)
	if err != nil {
		_ = lock.Release()
		_ = closeLog()
		return nil, fmt.Errorf("db open: %w", err)
	}
	if err := persistence.RunMigrations(db); err != nil {
		_ = db.Close()
		_ = lock.Release()
		_ = closeLog()
		return nil, fmt.Errorf("migrations: %w", err)
	}

	// 4. ハンドラ配線
	configStore := persistence.NewConfigStoreSQL(db)
	configUC := usecase.NewConfigUseCase(configStore)
	configHandler := handler.NewConfigHandler(configUC)

	lg.Info("bootstrap complete", "db", dbPath, "logDir", logDir)

	return &Services{
		DB:            db,
		Logger:        lg,
		LoggerClose:   closeLog,
		Lock:          lock,
		ConfigHandler: configHandler,
	}, nil
}

// Close は Services が保持する全リソースを開放する。
func (s *Services) Close() {
	if s.DB != nil {
		_ = s.DB.Close()
	}
	if s.Lock != nil {
		_ = s.Lock.Release()
	}
	if s.LoggerClose != nil {
		_ = s.LoggerClose()
	}
}
