# BMS Random Table Compositor

## プロジェクト概要

既存BMS難易度表をローカルで再ホストし、ランダム選曲・所持限定・合成等の編集を加えて beatoraja に提供するWindows向けデスクトップアプリ。
詳細は `docs/superpowers/specs/2026-05-06-bms-random-table-compositor-design.md` 参照。

## ビルド
- コンパイル確認には `go build ./...` を使う（`go build .` はバイナリ出力するため不可）

## マイグレーション
- スキーマ変更は `internal/adapter/persistence/migrations.go` に冪等な `CREATE IF NOT EXISTS` / `ALTER TABLE` として追加
- 既存DBが壊れないよう `pragma_table_info` チェックで保護
- 必ずユニットテストを追加 (`internal/adapter/persistence/migrations_test.go`)

## フロントエンド
- Stack: Svelte 4 + Vite 5 + TypeScript 5 + Tailwind v4 + daisyUI v5 (theme: emerald)。bms-elsa の構成を参考にする

## テストデータ
- `testdata/{songdata.db,satellite_*.json}` は `.gitignore` 対象 (PUBLIC リポジトリで個人/他者データ非載せ)
- `internal/adapter/persistence/songdata_reader_test.go` は `testdata/songdata.db` をローカル配置前提。クリーンチェックアウト直後はこのテストだけ失敗する

## マニュアル
- ユーザー向けは `docs/manual.md`。機能追加・変更時は更新
