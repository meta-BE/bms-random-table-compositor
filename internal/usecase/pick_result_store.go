package usecase

import (
	"sync"

	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
)

// PickResultStore は in-memory のピック結果キャッシュ。プロセス再起動で消える。
type PickResultStore struct {
	mu sync.RWMutex
	m  map[string]domain.PickResult
}

// NewPickResultStore は新しい PickResultStore を作る。
func NewPickResultStore() *PickResultStore {
	return &PickResultStore{m: map[string]domain.PickResult{}}
}

// Get は publishedID のピック結果を返す。
func (s *PickResultStore) Get(publishedID string) (domain.PickResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.m[publishedID]
	return r, ok
}

// Set はピック結果を保存する。
func (s *PickResultStore) Set(publishedID string, r domain.PickResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[publishedID] = r
}

// Delete は publishedID のピック結果を削除する。存在しなくてもエラーにしない。
func (s *PickResultStore) Delete(publishedID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, publishedID)
}

// Snapshot は現在のキャッシュをコピーして返す（Plan 4 ダッシュボード表示用の前準備）。
func (s *PickResultStore) Snapshot() map[string]domain.PickResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]domain.PickResult, len(s.m))
	for k, v := range s.m {
		out[k] = v
	}
	return out
}

// Clear は全エントリを削除する（設定一括変更時の InvalidateAll で使う）。
func (s *PickResultStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m = map[string]domain.PickResult{}
}
