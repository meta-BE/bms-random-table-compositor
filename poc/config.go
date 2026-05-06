package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type Config struct {
	Port int `json:"port"`
}

const defaultPort = 50000

// configPath は実行ファイル隣の poc-config.json を返す。
// wails dev 時など実行ファイルが取れない/一時的な場合はカレントディレクトリにフォールバックする。
func configPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "poc-config.json"
	}
	return filepath.Join(filepath.Dir(exe), "poc-config.json")
}

// LoadConfig は config を読み込む。ファイルが無ければデフォルト値を返す。
func LoadConfig() (Config, error) {
	path := configPath()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{Port: defaultPort}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, err
	}
	if c.Port == 0 {
		c.Port = defaultPort
	}
	return c, nil
}

// SaveConfig は config を JSON でディスクに保存する。
func SaveConfig(c Config) error {
	path := configPath()
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
