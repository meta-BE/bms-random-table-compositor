package port

// IDGenerator は ULID 等のユニーク ID を生成する。
// usecase 層はこのインタフェース経由で ID を取得し、テストではフェイクで決定論化する。
type IDGenerator interface {
	New() string
}
