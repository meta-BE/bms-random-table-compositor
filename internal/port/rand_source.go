package port

// RandSource は math/rand.Source 互換の最低限のインタフェース。
// 決定論テストで Int63 / Seed をモック化するために導入。
type RandSource interface {
	Int63() int64
	Seed(seed int64)
}

// RandSourceFactory は seed を受け取って RandSource を作る関数型。
// PickUseCase に注入することで、テストで「常に同じ並び順を返す」モックに差し替えられる。
type RandSourceFactory func(seed int64) RandSource
