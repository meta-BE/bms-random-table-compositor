package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// Server はPOC用ローカルHTTPサーバ。Start/Stopでライフサイクルを制御する。
type Server struct {
	mu     sync.Mutex
	server *http.Server
	port   int
}

func NewServer() *Server {
	return &Server{}
}

// Start は指定ポートでHTTPサーバをgoroutineで起動する。
// ポート確保失敗時はerrorを返す（Listenが同期的なため、起動失敗を呼び出し元で捕捉できる）。
func (s *Server) Start(port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server != nil {
		return errors.New("server already running")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"hello": "world",
			"port":  port,
		})
	})

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("ポート %d の確保に失敗: %w", port, err)
	}

	s.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       2 * time.Second,
	}
	s.port = port

	go func() {
		_ = s.server.Serve(ln)
	}()
	return nil
}

// Stop はサーバをグレースフル停止する。未起動なら何もしない。
// ctxタイムアウト時は強制クローズにフォールバックする。
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	if s.server == nil {
		s.mu.Unlock()
		return nil
	}
	srv := s.server
	s.server = nil
	s.port = 0
	s.mu.Unlock()

	if err := srv.Shutdown(ctx); err != nil {
		// ctx タイムアウト等で graceful shutdown が間に合わない場合は強制終了
		_ = srv.Close()
		return err
	}
	return nil
}

// Running は現在サーバが起動中かを返す。
func (s *Server) Running() (bool, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.server != nil, s.port
}
