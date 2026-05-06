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
- 設定画面のUI規約は `docs/style-guide.md` に従う（後続 Plan で整備）

## マニュアル
- ユーザー向けは `docs/manual.md`（後続 Plan で整備）。機能追加・変更時は更新

## POC
- `poc/` 配下は Phase 0 POC の参照用コードベース。**本体実装はリポジトリルート直下**
- POC で得た知見は `poc/NOTES.md` を参照
