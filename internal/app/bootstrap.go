// Package app は Wails Bind ターゲットとなるハンドラ群と、サービス起動の配線を提供する。
package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/clock"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/gateway"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/httpserver"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/idgen"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/logger"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/paths"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/persistence"
	"github.com/meta-BE/bms-random-table-compositor/internal/adapter/randsrc"
	"github.com/meta-BE/bms-random-table-compositor/internal/app/handler"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

// Services はアプリ全体で共有する依存を保持する。
type Services struct {
	DB                    *sql.DB
	Logger                *slog.Logger
	LoggerClose           logger.CloseFunc
	ConfigHandler         *handler.ConfigHandler
	SourceTableHandler    *handler.SourceTableHandler
	PublishedTableHandler *handler.PublishedTableHandler
	PickHandler           *handler.PickHandler
	ServerStatusHandler   *handler.ServerStatusHandler
	OwnedChartHandler     *handler.OwnedChartHandler
	SourceTableUseCase    *usecase.SourceTableUseCase
	ServerUseCase         *usecase.ServerUseCase
}

// Bootstrap は Services を構築する（DB接続・マイグレーション・ロガー・各UseCase初期化）。
// シングルインスタンス制御は Wails の SingleInstanceLock オプションに任せる。
func Bootstrap() (*Services, error) {
	// 1. Logger
	logDir, err := paths.LogDir()
	if err != nil {
		return nil, fmt.Errorf("log dir: %w", err)
	}
	lg, closeLog, err := logger.New(logger.Options{
		LogDir: logDir, MaxSizeMB: 50, MaxBackups: 7, MaxAgeDays: 7,
	})
	if err != nil {
		return nil, fmt.Errorf("logger init: %w", err)
	}

	// 2. DB と マイグレーション
	dbPath, err := paths.DBPath()
	if err != nil {
		_ = closeLog()
		return nil, fmt.Errorf("db path: %w", err)
	}
	db, err := persistence.OpenDB(dbPath)
	if err != nil {
		_ = closeLog()
		return nil, fmt.Errorf("db open: %w", err)
	}
	if err := persistence.RunMigrations(db); err != nil {
		_ = db.Close()
		_ = closeLog()
		return nil, fmt.Errorf("migrations: %w", err)
	}

	// 3. UseCase / Handler 配線
	configStore := persistence.NewConfigStoreSQL(db)
	configUC := usecase.NewConfigUseCase(configStore)
	configHandler := handler.NewConfigHandler(configUC)

	sourceRepo := persistence.NewSourceTableRepoSQL(db)
	httpClient := &http.Client{Timeout: 30 * time.Second}
	fetcher := gateway.NewBMSTableFetcher(httpClient, lg)
	idGen := idgen.NewULID()
	sourceUC := usecase.NewSourceTableUseCase(sourceRepo, fetcher, idGen, lg)
	sourceHandler := handler.NewSourceTableHandler(sourceUC)

	pubRepo := persistence.NewPublishedTableRepoSQL(db)
	pubUC := usecase.NewPublishedTableUseCase(pubRepo, sourceRepo, idGen, lg)
	pubHandler := handler.NewPublishedTableHandler(pubUC)

	systemClock := clock.System{}
	ownedRepo := persistence.NewSongdataReader()
	ownedCache := usecase.NewOwnedMD5Cache(ownedRepo, configStore, systemClock, lg)
	pickStore := usecase.NewPickResultStore()
	randFactory := port.RandSourceFactory(func(seed int64) port.RandSource {
		return randsrc.NewMathRandSource(seed)
	})
	pickUC := usecase.NewPickUseCase(pubRepo, sourceRepo, ownedCache, pickStore, systemClock, randFactory, lg)
	pickHandler := handler.NewPickHandler(pickUC)
	ownedHandler := handler.NewOwnedChartHandler(ownedCache)

	// songdata_db_path 変更時に owned cache を invalidate + ピックキャッシュを clear
	configUC.AddSongdataPathChangeHook(func() {
		ownedCache.Invalidate()
		pickUC.InvalidateAll()
	})

	httpFactory := func(addr string) usecase.HTTPServer {
		return httpserver.New(addr, httpserver.Deps{Pick: pickUC, Pub: pubUC, Log: lg})
	}
	serverUC := usecase.NewServerUseCase(configStore, httpFactory, lg)
	serverHandler := handler.NewServerStatusHandler(serverUC)

	lg.Info("bootstrap complete", "db", dbPath, "logDir", logDir)

	return &Services{
		DB:                    db,
		Logger:                lg,
		LoggerClose:           closeLog,
		ConfigHandler:         configHandler,
		SourceTableHandler:    sourceHandler,
		PublishedTableHandler: pubHandler,
		PickHandler:           pickHandler,
		ServerStatusHandler:   serverHandler,
		OwnedChartHandler:     ownedHandler,
		SourceTableUseCase:    sourceUC,
		ServerUseCase:         serverUC,
	}, nil
}

// Close は Services が保持する全リソースを解放する。
// サーバ稼働中は最大 5 秒で graceful shutdown する。
func (s *Services) Close() {
	if s.ServerUseCase != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.ServerUseCase.Stop(ctx)
	}
	if s.DB != nil {
		_ = s.DB.Close()
	}
	if s.LoggerClose != nil {
		_ = s.LoggerClose()
	}
}
