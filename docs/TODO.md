# TODO / v2 以降の機能候補

Phase 1 MVP (v0.1.0, 2026-05-08) 完了後の機能候補。
着手する際は `/brainstorming` で 1 つ選んで掘り下げ、必要なら Plan ドキュメントを `docs/superpowers/specs/` に追加する。

## v2 機能

- ✅ **複数ソース表合成** (v0.2.0 で実装): 複数の難易度表を 1 つの公開表にマージ。詳細: `docs/superpowers/specs/2026-05-10-multi-source-table-composition-design.md`
- **ピックアルゴリズム B/C**: 現状の per_request / daily / manual に加える新方式
- **最終プレイ日時優先**: `scorelog.db` 等を参照してピックに反映
- **ETag 304 本格運用**: HTTP キャッシュ最適化
- **自動スケジュール更新**: ソース表のバックグラウンド定期更新
- **PR 自動 CI**: GitHub Actions 拡充
- **自動リリース**: タグ手動運用からの脱却

## MVP 成熟化

- スクリーンショット入りマニュアル
- IPv4 自接続テスト
- Tray 状態に応じた高度な振る舞い (アイコンデザイン本格化)

## 運用

- 上記に着手せず Phase 1 の運用に入る選択肢もあり (一区切り)
