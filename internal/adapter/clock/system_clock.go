// Package clock は port.Clock の素朴実装を提供する。
package clock

import "time"

// System は time.Now() をそのまま返す port.Clock 実装。
type System struct{}

// Now は現在時刻を返す。
func (System) Now() time.Time { return time.Now() }
