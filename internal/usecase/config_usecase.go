package usecase

import (
	"context"
	"fmt"
	"strconv"

	"github.com/meta-BE/bms-random-table-compositor/internal/port"
)

// 既知の config キー
const (
	keyServerPort     = "server_port"
	keySongdataDBPath = "songdata_db_path"
	defaultServerPort = 50000
)

// ConfigUseCase は config の Get/Set を型安全にラップする。
type ConfigUseCase struct {
	store port.ConfigStore
}

// NewConfigUseCase は新しい ConfigUseCase を作る。
func NewConfigUseCase(store port.ConfigStore) *ConfigUseCase {
	return &ConfigUseCase{store: store}
}

// GetServerPort は HTTP サーバのポート番号を返す。未設定時は defaultServerPort。
func (u *ConfigUseCase) GetServerPort(ctx context.Context) (int, error) {
	v, found, err := u.store.Get(ctx, keyServerPort)
	if err != nil {
		return 0, err
	}
	if !found || v == "" {
		return defaultServerPort, nil
	}
	port, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("parse server_port %q: %w", v, err)
	}
	return port, nil
}

// SetServerPort は HTTP サーバのポート番号を保存する。範囲は 1〜65535。
func (u *ConfigUseCase) SetServerPort(ctx context.Context, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("ポート番号は 1〜65535 の範囲で指定してください (got %d)", port)
	}
	return u.store.Set(ctx, keyServerPort, strconv.Itoa(port))
}

// GetSongdataDBPath は beatoraja の songdata.db のパスを返す。未設定時は空文字列。
func (u *ConfigUseCase) GetSongdataDBPath(ctx context.Context) (string, error) {
	v, _, err := u.store.Get(ctx, keySongdataDBPath)
	if err != nil {
		return "", err
	}
	return v, nil
}

// SetSongdataDBPath は songdata.db のパスを保存する（バリデーションは行わない）。
func (u *ConfigUseCase) SetSongdataDBPath(ctx context.Context, path string) error {
	return u.store.Set(ctx, keySongdataDBPath, path)
}
