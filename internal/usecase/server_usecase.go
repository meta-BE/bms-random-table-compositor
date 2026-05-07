package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
	"github.com/meta-BE/bms-random-table-compositor/internal/port"
)

// HTTPServer は usecase 層が触る HTTP サーバの抽象。
// adapter/httpserver が実装する。
type HTTPServer interface {
	Start() error                       // Listen + 非同期 Serve（Listen 失敗時のみ error）
	Shutdown(ctx context.Context) error // graceful shutdown
	Addr() string                       // 例: ":50000"
}

// HTTPServerFactory は addr を受け取って HTTPServer を作る。
type HTTPServerFactory func(addr string) HTTPServer

// ServerUseCase はサーバの起動 / 停止 / 再起動 / ステータスを管理する。
type ServerUseCase struct {
	cfg     port.ConfigStore
	factory HTTPServerFactory
	log     *slog.Logger

	mu        sync.Mutex
	status    domain.ServerStatus
	server    HTTPServer
	listeners []func(domain.ServerStatus)
}

// NewServerUseCase は新しい ServerUseCase を作る。
func NewServerUseCase(cfg port.ConfigStore, factory HTTPServerFactory, log *slog.Logger) *ServerUseCase {
	return &ServerUseCase{
		cfg: cfg, factory: factory, log: log,
		status: domain.ServerStatus{State: domain.ServerStateStopped},
	}
}

// OnStatusChange はステータス変化を購読する。Plan 4 でトレイから購読する想定。
// 同期的に呼ばれるため、リスナー側は重い処理をしないか自分で goroutine 化すること。
func (u *ServerUseCase) OnStatusChange(fn func(domain.ServerStatus)) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.listeners = append(u.listeners, fn)
}

// Status は現在のサーバステータスを返す。
func (u *ServerUseCase) Status() domain.ServerStatus {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.status
}

// Start はサーバを起動する。既に起動中なら ErrServerAlreadyRunning。
func (u *ServerUseCase) Start(ctx context.Context) error {
	u.mu.Lock()
	if u.status.State == domain.ServerStateRunning {
		u.mu.Unlock()
		return ErrServerAlreadyRunning
	}
	u.mu.Unlock()

	port, err := u.readPort(ctx)
	if err != nil {
		u.setStatusError(fmt.Sprintf("ポート設定エラー: %v", err))
		return err
	}

	srv := u.factory(fmt.Sprintf(":%d", port))
	if err := srv.Start(); err != nil {
		u.setStatusError(err.Error())
		u.log.Warn("server start failed", "port", port, "err", err)
		return err
	}
	now := time.Now()
	u.mu.Lock()
	u.server = srv
	u.status = domain.ServerStatus{
		State: domain.ServerStateRunning, Port: port, StartedAt: &now,
	}
	listeners := append(([]func(domain.ServerStatus))(nil), u.listeners...)
	u.mu.Unlock()
	u.log.Info("server started", "port", port)
	notify(listeners, u.Status())
	return nil
}

// Stop はサーバを停止する。停止中なら ErrServerNotRunning。
func (u *ServerUseCase) Stop(ctx context.Context) error {
	u.mu.Lock()
	if u.status.State != domain.ServerStateRunning {
		u.mu.Unlock()
		return ErrServerNotRunning
	}
	srv := u.server
	u.mu.Unlock()

	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		u.log.Warn("server shutdown error", "err", err)
		// ステータスは stopped に倒す（資源は最善努力で解放済みとみなす）
	}
	u.mu.Lock()
	u.server = nil
	u.status = domain.ServerStatus{State: domain.ServerStateStopped}
	listeners := append(([]func(domain.ServerStatus))(nil), u.listeners...)
	u.mu.Unlock()
	u.log.Info("server stopped")
	notify(listeners, u.Status())
	return nil
}

// Restart は Stop → Start。起動中でなければ Start のみ実行。
func (u *ServerUseCase) Restart(ctx context.Context) error {
	if u.Status().State == domain.ServerStateRunning {
		if err := u.Stop(ctx); err != nil {
			return err
		}
	}
	return u.Start(ctx)
}

func (u *ServerUseCase) readPort(ctx context.Context) (int, error) {
	v, _, err := u.cfg.Get(ctx, "server_port")
	if err != nil {
		return 0, err
	}
	if v == "" {
		return 50000, nil
	}
	p, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid server_port %q: %w", v, err)
	}
	return p, nil
}

func (u *ServerUseCase) setStatusError(msg string) {
	u.mu.Lock()
	u.status = domain.ServerStatus{State: domain.ServerStateError, LastError: msg}
	listeners := append(([]func(domain.ServerStatus))(nil), u.listeners...)
	u.mu.Unlock()
	notify(listeners, u.Status())
}

func notify(fns []func(domain.ServerStatus), s domain.ServerStatus) {
	for _, fn := range fns {
		fn(s)
	}
}
