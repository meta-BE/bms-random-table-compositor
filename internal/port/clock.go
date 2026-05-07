package port

import "time"

// Clock は現在時刻を返す。テストで固定する目的で抽象化する。
type Clock interface {
	Now() time.Time
}
