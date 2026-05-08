// Package httpserver は public な /:slug 系エンドポイントを提供する HTTP サーバの adapter。
package httpserver

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/meta-BE/bms-random-table-compositor/internal/usecase"
)

//go:embed templates/index.html
var templatesFS embed.FS

var indexTemplate = template.Must(template.ParseFS(templatesFS, "templates/index.html"))

// Deps は HTTP ハンドラが依存する usecase 群。
type Deps struct {
	Pick      *usecase.PickUseCase
	Pub       *usecase.PublishedTableUseCase
	Dashboard *usecase.DashboardUseCase
	Log       *slog.Logger
}

// AdapterServer は usecase.HTTPServer を実装する *http.Server ラッパ。
type AdapterServer struct {
	addr string
	srv  *http.Server
	mu   sync.Mutex
	ln   net.Listener
	done chan struct{}
}

// New は addr (":50000" 等) と Deps を受け取り AdapterServer を作る。
func New(addr string, deps Deps) *AdapterServer {
	mux := NewMux(deps)
	return &AdapterServer{
		addr: addr,
		srv: &http.Server{
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}

// Addr は listen アドレスを返す。
func (s *AdapterServer) Addr() string { return s.addr }

// Start は同期的に Listen して、Serve は goroutine で起動する。
// Listen 失敗時のみエラーを返す（Serve のエラーはログのみ）。
func (s *AdapterServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.addr, err)
	}
	s.ln = ln
	s.done = make(chan struct{})
	go func() {
		defer close(s.done)
		if err := s.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Default().Warn("http server Serve exited", "err", err)
		}
	}()
	return nil
}

// Shutdown は graceful shutdown する。
func (s *AdapterServer) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	srv := s.srv
	done := s.done
	s.mu.Unlock()
	if srv == nil {
		return nil
	}
	if err := srv.Shutdown(ctx); err != nil {
		return err
	}
	if done != nil {
		<-done
	}
	return nil
}
