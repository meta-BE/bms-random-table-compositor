package usecase

import (
	"sync"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
)

// PickResultStore は in-memory のピック結果キャッシュ。プロセス再起動で消える。
type PickResultStore struct {
	mu        sync.RWMutex
	m         map[string]domain.PickResult
	listeners []func(publishedID string)
}

// NewPickResultStore は新しい PickResultStore を作る。
func NewPickResultStore() *PickResultStore {
	return &PickResultStore{m: map[string]domain.PickResult{}}
}

// OnChange は Set / Delete / Clear 時に呼ばれるリスナーを登録する。
// Clear は publishedID="" で通知。同期的に呼ばれるので、リスナー側は重い処理をしないか
// 自分で goroutine 化する。
func (s *PickResultStore) OnChange(fn func(publishedID string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = append(s.listeners, fn)
}

func (s *PickResultStore) notify(publishedID string) {
	// listeners のコピーを取ってからロック解除して呼ぶ (デッドロック回避)
	s.mu.Lock()
	listeners := append(([]func(string))(nil), s.listeners...)
	s.mu.Unlock()
	for _, fn := range listeners {
		fn(publishedID)
	}
}

// Get は publishedID のピック結果を返す。
func (s *PickResultStore) Get(publishedID string) (domain.PickResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.m[publishedID]
	return r, ok
}

// Set はピック結果を保存し、リスナーに通知する。
func (s *PickResultStore) Set(publishedID string, r domain.PickResult) {
	s.mu.Lock()
	s.m[publishedID] = r
	s.mu.Unlock()
	s.notify(publishedID)
}

// Delete は publishedID のピック結果を削除し、リスナーに通知する。存在しなくてもエラーにしない。
func (s *PickResultStore) Delete(publishedID string) {
	s.mu.Lock()
	delete(s.m, publishedID)
	s.mu.Unlock()
	s.notify(publishedID)
}

// Snapshot は現在のキャッシュをコピーして返す (Plan 4 ダッシュボード表示用)。
func (s *PickResultStore) Snapshot() map[string]domain.PickResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]domain.PickResult, len(s.m))
	for k, v := range s.m {
		out[k] = v
	}
	return out
}

// Clear は全エントリを削除し、リスナーに publishedID="" で通知する (設定一括変更時の InvalidateAll で使う)。
func (s *PickResultStore) Clear() {
	s.mu.Lock()
	s.m = map[string]domain.PickResult{}
	s.mu.Unlock()
	s.notify("")
}
