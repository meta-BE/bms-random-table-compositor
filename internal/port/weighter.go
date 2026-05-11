package port

import (
	"github.com/meta-BE/bms-random-table-compositor/internal/domain"
)

// Weighter はピック時の重み関数。集合内で正規化された経過時間 a ∈ [0, 1]
// (0 = 最新プレイ, 1 = 最古プレイ / 未プレイ) を重みに変換する純関数として実装する。
// 0 以下を返した譜面は対象外として扱う。
// 正規化スコープと a の計算は呼出側 (pickLevel) の責務。
type Weighter interface {
	Weight(a float64) float64
}

// WeighterFactory は PickConfig から適切な Weighter を選択する。
// Bootstrap で具体実装を 1 つだけ注入し、PickUseCase が公開表ごとに For を呼ぶ。
type WeighterFactory interface {
	For(cfg domain.PickConfig) Weighter
}
