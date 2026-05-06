package port

import "context"

// ConfigStore は config テーブルへの key-value ストアアクセスを提供する。
type ConfigStore interface {
	// Get は指定キーの値を返す。存在しない場合 found=false を返す。
	Get(ctx context.Context, key string) (value string, found bool, err error)
	// Set は指定キーに値を保存する。既存キーは上書きされる。
	Set(ctx context.Context, key string, value string) error
}
