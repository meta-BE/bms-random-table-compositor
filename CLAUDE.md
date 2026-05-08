# BMS Random Table Compositor

## プロジェクト概要

既存BMS難易度表をローカルで再ホストし、ランダム選曲・所持限定・合成等の編集を加えて beatoraja に提供するWindows向けデスクトップアプリ。
詳細は `docs/superpowers/specs/2026-05-06-bms-random-table-compositor-design.md` 参照。

## コマンド
- `make dev` : Wails 開発モード起動 (Vite ホットリロード + Go バックエンド)
- `make build` / `make build-windows` : 本番ビルド
- `make test` : 全 Go テスト
- `make lint` : `go vet` + `gofmt` チェック
- フロントエンド型チェック: `cd frontend && npm run check`

## アーキテクチャ
Hexagonal Architecture を採用 (`internal/` 配下):
- `domain/` : ドメインモデル (純粋ロジック、外部依存なし)
- `port/` : ユースケースが要求するインターフェース定義
- `usecase/` : アプリケーションロジック (port を受け取って動作)
- `adapter/` : port の実装 (persistence, httpserver, gateway, tray, etc.)
- `app/` : DI (`Bootstrap`) と Wails ハンドラ (`handler/`)
- `main.go` / `app.go` : Wails エントリポイント (services を Bind)

## ビルド
- コンパイル確認には `go build ./...` を使う（`go build .` はバイナリ出力するため不可）

## Wails バインド
- Go ハンドラのメソッドシグネチャを変えたら `wails generate module` でフロント側 (`frontend/wailsjs/`) を再生成する
- `frontend/wailsjs/` は `.gitignore` 対象 (`make dev` / `wails build` 時に自動生成)

## マイグレーション
- スキーマ変更は `internal/adapter/persistence/migrations.go` に冪等な `CREATE IF NOT EXISTS` / `ALTER TABLE` として追加
- 既存DBが壊れないよう `pragma_table_info` チェックで保護
- 必ずユニットテストを追加 (`internal/adapter/persistence/migrations_test.go`)

## フロントエンド
- Stack: Svelte 4 + Vite 5 + TypeScript 5 + Tailwind v4 + daisyUI v5 (theme: emerald)。bms-elsa の構成を参考にする

## テストデータ
- `testdata/{songdata.db,satellite_*.json}` は `.gitignore` 対象 (PUBLIC リポジトリで個人/他者データ非載せ)
- `internal/adapter/persistence/songdata_reader_test.go` は `testdata/songdata.db` をローカル配置前提。クリーンチェックアウト直後はこのテストだけ失敗する
- 上記を回避して全テストを通したい場合: `go test $(go list ./... | grep -v internal/adapter/persistence)` または該当テストを `-run` で除外

## マニュアル
- ユーザー向けは `docs/manual.md`。機能追加・変更時は更新
