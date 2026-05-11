package weighter

// LastPlayedWeighter は最終プレイ日時に基づく線形補間の重み関数。
// 集合内正規化経過時間 a ∈ [0,1] (0=最新, 1=最古) を入力に
// w = 1 + (X-1)*a を返す。X=1 で恒等 (一様), X=K で「最古は最新の K 倍」。
type LastPlayedWeighter struct {
	X float64
}

func (w LastPlayedWeighter) Weight(a float64) float64 {
	return 1.0 + (w.X-1.0)*a
}
