# Phase 1 MVP 手動 E2E テスト計画

最終更新: 2026-05-08 (Plan 4)

## 環境

- macOS と Windows の両方で実施
- 既存の `compositor.db` を退避してクリーン起動 (例: `compositor.db.bak` にリネーム)
- beatoraja は別途インストール済みで `songdata.db` のパスが分かっていること

## チェックリスト

### 起動・常駐 (Windows メイン)

- [ ] アプリ起動でメインウィンドウが表示される
- [ ] ウィンドウクローズでトレイに格納される (Windows / Linux のみ)
- [ ] トレイメニュー「設定を開く」「終了」が動く
- [ ] 二重起動で既存ウィンドウが前面化される
- [ ] トレイアイコンがサーバ状態に応じて 3 色で切り替わる (停止: グレー / 起動中: 緑 / エラー: 赤)
- [ ] macOS ではウィンドウクローズで通常終了する

### サーバ設定タブ

- [ ] 初回起動時、ポート 50000 / songdata.db パス空 で表示される
- [ ] ポート変更 → 保存 → 「再起動」で反映される
- [ ] songdata.db を「参照…」ボタンで OS ファイル選択ダイアログから指定できる
- [ ] サーバ「停止」「起動」「再起動」が状態に応じて enable/disable される
- [ ] 起動中は緑バッジ + ポート + 起動時刻 (JST) が表示される
- [ ] エラー時は赤バッジ + エラーメッセージ alert が表示される
- [ ] 所持キャッシュ「再読み込み」ボタンで count が更新される
- [ ] songdata.db パス変更後、所持キャッシュが invalidate される

### ソース表タブ

- [ ] HTML URL (例: stellabms.xyz の SL 表) で追加 → バックグラウンド更新で行が入る
- [ ] header.json 直 URL で追加 → 同上
- [ ] 取得失敗 (例: 不正な URL) 時に `badge-error` + エラー本文が表示される
- [ ] 行右クリック → コンテキストメニュー (再取得 / 削除)
- [ ] 削除で ConfirmDialog (キャンセル / 削除) が出る
- [ ] 「一括再取得」ボタンで全ソース表が同時に再取得される
- [ ] 個別「再取得」ボタンの動作中はその行のスピナーが出る
- [ ] 0 件状態でプレースホルダ ("ソース表が登録されていません") が出る

### 公開表タブ

- [ ] ソース表 0 件のとき新規作成ボタンが disabled、警告 alert が出る
- [ ] CRUD (新規作成 / 編集 / 削除) が動く
- [ ] 既存行を編集 → slug 未変更で保存できる (Plan 4 で fix した bug)
- [ ] slug 自動生成ボタンが動く
- [ ] slug 形式不正 / 予約語 / 重複の各エラーが赤字で出る
- [ ] 削除で ConfirmDialog が出る
- [ ] 行右クリック → コンテキストメニュー (4 項目)
- [ ] `refresh_mode != manual` の行で「再ピック」が disabled
- [ ] 「ブラウザで開く」で `http://127.0.0.1:<port>/<slug>` がデフォルトブラウザで開く

### ダッシュボードタブ

- [ ] 初期表示で Snapshot が出る (起動直後は空、RefreshAll 完了後にフェッチが入る)
- [ ] 別タブでソース表追加 → ダッシュボードタブの「ソース表更新履歴」がリアルタイム更新
- [ ] ブラウザで `http://127.0.0.1:<port>/<slug>` にアクセス → 「最近のリクエスト」がリアルタイム更新
- [ ] 「再ピック」 → 「現在のピック結果」が更新
- [ ] 100 件超えで古い行が捨てられる (連続 105 リクエスト等)
- [ ] 0 件状態でプレースホルダが出る
- [ ] 時刻が JST で表示される

### HTTP エンドポイント

- [ ] `GET /<slug>` HTML ビューが表示される (所持/未所持で色分け)
- [ ] OwnedOnly=false 公開表でも実所持で色分けされる (所持あり/なしの両方の譜面が混じる場合)
- [ ] OwnedOnly=true 公開表は全件 owned 表示
- [ ] `GET /<slug>/header.json` の `level_order` が実在レベルのみ
- [ ] `GET /<slug>/data.json` の各モード:
  - per_request: 連続リクエストで結果が変わる
  - daily: 同一日付内で固定
  - manual: 「再ピック」ボタンまで固定
- [ ] `POST /<slug>/_refresh` は manual モードのみ受付、それ以外は 405
- [ ] 存在しない slug は 404
- [ ] HTML ビュー内の `<meta name="bmstable">` が `/{slug}/header.json` (絶対パス) になっている

### beatoraja 接続

- [ ] beatoraja の難易度表 URL に `http://127.0.0.1:<port>/<slug>` を登録 → 譜面一覧が出る
- [ ] OwnedOnly=true 公開表で所持譜面のみが出る
- [ ] OwnedOnly=false 公開表で全譜面が出る
- [ ] manual モード公開表で beatoraja 側からの再読み込みでも結果が変わらない
- [ ] daily モードの日付跨ぎで結果が変わる (時計を 1 日進めて再アクセス)

### 無回帰 (Plan 1-3)

- [ ] 単一インスタンスロックが効く (二重起動で既存窓が前面化)
- [ ] OnBeforeClose で WindowHide される (Win/Linux)
- [ ] `compositor.db` のマイグレーションが冪等 (再起動で問題なし)
- [ ] ログが `logs/YYYY-MM-DD.log` に出力される

## Windows ビルド・実機確認手順

1. `git push origin main` (Plan 2 lessons #5 通り、リモート HEAD を Windows runner が見るため必須)
2. `gh workflow run build-windows.yml --ref main`
3. `gh run list --workflow=build-windows.yml --limit 1` で run-id 確認
4. `gh run watch <run-id>` で完了待機
5. `gh run download <run-id> --name <artifact-name> --dir ./tmp/win`
6. Windows 機 / VM で `bms-random-table-compositor.exe` を起動して上記チェックリストを再実行
