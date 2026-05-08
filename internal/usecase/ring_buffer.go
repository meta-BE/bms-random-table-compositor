package usecase

import "sync"

// RingBuffer は容量上限付きの単純なリングバッファ。スレッドセーフ。
// Snapshot は新しい順にコピーした slice を返す (元の格納順は古→新)。
type RingBuffer[T any] struct {
	mu   sync.RWMutex
	cap  int
	data []T
}

// NewRingBuffer は capacity 件の容量を持つリングバッファを作る。
func NewRingBuffer[T any](capacity int) *RingBuffer[T] {
	if capacity < 1 {
		capacity = 1
	}
	return &RingBuffer[T]{cap: capacity, data: make([]T, 0, capacity)}
}

// Append は要素を追加する。容量超過時は最古の要素が捨てられる。
func (r *RingBuffer[T]) Append(v T) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.data) >= r.cap {
		r.data = r.data[1:]
	}
	r.data = append(r.data, v)
}

// Snapshot は現在の格納要素を新しい順 (新→古) にコピーして返す。
// 結果スライスは呼び出し側が自由に変更してよい。
func (r *RingBuffer[T]) Snapshot() []T {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]T, len(r.data))
	for i, v := range r.data {
		out[len(r.data)-1-i] = v
	}
	return out
}
