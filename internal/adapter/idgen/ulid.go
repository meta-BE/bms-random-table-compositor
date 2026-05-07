// Package idgen は port.IDGenerator の adapter 実装を提供する。
package idgen

import (
	"crypto/rand"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// ULIDGenerator は crypto/rand エントロピーで ULID を生成する。
// MonotonicReader を使うため同一ミリ秒内でも単調増加し、ロック付きで並行安全。
type ULIDGenerator struct {
	mu      sync.Mutex
	entropy *ulid.MonotonicEntropy
}

// NewULID は本番用の ULIDGenerator を返す。エントロピー源は crypto/rand。
func NewULID() *ULIDGenerator {
	return &ULIDGenerator{
		entropy: ulid.Monotonic(rand.Reader, 0),
	}
}

// New は新しい ULID 文字列（26 文字、Crockford Base32）を返す。
func (g *ULIDGenerator) New() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return ulid.MustNew(ulid.Timestamp(time.Now()), g.entropy).String()
}
