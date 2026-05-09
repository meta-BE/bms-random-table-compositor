---
name: update-release-notes
description: |
  GitHub Releases の body にリリースノートを生成・更新する。
  「リリースノート作成して」「v0.11.2 のリリースノートを作って」
  「最新リリースのノートを更新」などの指示があった場合に使用。
model: claude-sonnet-4-6
allowed-tools:
  - Bash
  - Write
---

# リリースノート生成・更新ガイドライン

GitHub Releases の body に、コミットログから生成したリリースノート（`## 新機能` / `## バグ修正` / `## その他の変更`）を埋めるスキル。

タグさえ存在すれば生成パートは進められる設計とし、Release 未作成 (CI ビルド中など) のときも待ち時間中にコミット範囲取得・分類・文章生成を完了させる。

## 引数

- 引数なし → 最新 GitHub Release を対象
- 引数あり（`v0.11.2` または `0.11.2`）→ 指定リリースを対象

## 実行手順

### 1. バージョン正規化

- 引数が `v` なし（例: `0.11.2`）の場合、`v` を付けて `v0.11.2` に正規化する
- 引数なしの場合、最新リリースを取得する：

```bash
gh release list --limit 1 --json tagName --jq '.[0].tagName'
```

### 2. 対象タグ存在確認 + リリース状態判定

タグの有無を git で先に確認する（Release 未作成でも生成は進められる）：

```bash
git rev-parse --verify "<tag>" 2>/dev/null
```

タグが存在しない場合は「タグ <tag> が見つかりません」と表示して停止する。

タグ存在後、Release 状態を確認する（この時点ではブロックしない）：

```bash
gh release view <tag> --json tagName,body 2>/dev/null
```

| 結果 | 扱い |
|---|---|
| Release 存在 | `body` を保持し §7 で上書き判定に使う。状態フラグ `release_pending=false` |
| Release 未作成 | 状態フラグ `release_pending=true` を立て、生成は続行。§8 で待機/作成判断 |

`release_pending=true` のとき、tag push に紐づく workflow が in_progress なら run URL をユーザーに提示しておく：

```bash
gh run list --event=push --limit 5 \
  --json status,databaseId,headBranch,url \
  --jq '.[] | select(.headBranch=="<tag>" and .status=="in_progress")'
```

### 3. 前バージョン解決

git tag を真実とする（Release 公開状況に依存させない）：

```bash
git tag --list 'v*' --sort=-version:refname
```

- このリストで対象タグの直後（= semver で 1 つ古い）を「前バージョン」とする
- 対象が最古のタグなら、前バージョン = リポジトリ最初のコミット：

```bash
git rev-list --max-parents=0 HEAD | head -1
```

### 4. コミット取得

```bash
git log <prev>..<target> --no-merges \
  --pretty=format:'%H%x09%s%x09%b%x1e'
```

- `<target>` は対象タグ
- `<prev>` は前バージョンのタグ、または最古リリースのときは最初のコミットハッシュ
- merge コミットは除外
- レコード区切りは `\x1e`、フィールド区切りは `\t`（コミット本文の改行を保持）

### 5. コミット分類

取得したコミット一覧を読み、次の4カテゴリに振り分ける。

| 分類 | 内容 |
|---|---|
| 新機能 | ユーザーから見える新しい機能の追加 |
| バグ修正 | ユーザーから見える不具合の修正 |
| その他の変更 | ユーザーから見える挙動変更・機能削除・破壊的変更など |
| 除外 | ドキュメント・内部リファクタ・テスト・CI・依存更新など、ユーザーに見えない変更 |

#### 判定の指針

- prefix（`feat:`/`fix:`/`refactor:` など）はヒントとして参照、最終判断はメッセージ全文の内容で行う
- 迷った場合は「ユーザー視点で挙動が変わるか」を基準にする
- 全コミットが「除外」になった場合は「ユーザー向けの変更がありません」と表示して停止（空のリリースノートで上書きしない）

### 6. 文章生成

#### スタイル規約

- 各項目は完結した文（体言止め禁止、「〜した」「〜を追加」「〜を修正」のような述語で締める）
- 背景・対象・挙動を含めて1〜2文で説明
- コードシンボルはバッククォートで囲む（例: `config.json`）
- 同じ機能領域に関する複数コミットは1項目にまとめる（バグ修正の積み重ねなど）

#### 過去文体の参照

過去リリースの body をいくつか参照して文体を揃える：

```bash
gh release view <prev_tag> --json body --jq .body
```

#### 出力フォーマット

```markdown
## 新機能
- ...

## バグ修正
- ...

## その他の変更
- ...
```

- 順序: `## 新機能` → `## バグ修正` → `## その他の変更`
- 該当項目がないセクションは出力しない

### 7. 上書き確認

- §2 で `release_pending=true` だった場合 → 上書き確認はスキップして §8 の作成パスへ
- §2 で取得した既存 body が空文字列（または空白・改行のみ）の場合 → 無確認で進む
- 何か入っている場合：
    - 既存 body と生成した新しい body をユーザーに表示する
    - 「上書きしますか？」と問い、明示的な承認（「はい」「ok」など）を待つ
    - 拒否されたら停止する

### 8. リリース更新

承認後（または既存 body が空、または Release 未作成）：

1. 生成テキストを一時ファイル `.96kudye/tmp/<YYMMDD>_release-notes-<tag>.md` に書き出す（`Write` ツール）
    - YYMMDD は環境情報の "Today's date" から取得する（推測禁止）
    - ディレクトリが無ければ作成する
    - 書き込みがブロックされた場合は `save-temp-file` スキルへ誘導する

2. Release 状態で分岐：

   **Release 存在 (`release_pending=false`)**:
   ```bash
   gh release edit <tag> --notes-file <file>
   ```

   **Release 未作成 (`release_pending=true`)**:
    - tag push 起点の CI run が in_progress なら待機オプションを提示し、ユーザー承認後に待機 → `gh release edit` を実行：
      ```bash
      gh run watch <run_id> --exit-status
      gh release edit <tag> --notes-file <file>
      ```
    - CI が存在しない／既に失敗している場合は `gh release create` で先行作成（CI が後から同タグの Release を上書きしないか要確認）：
      ```bash
      gh release create <tag> --notes-file <file>
      ```
    - 判断がつかない場合はユーザーへ手動対応を促して停止する

3. 更新後、URL を取得してユーザーに通知する：

```bash
gh release view <tag> --json url --jq .url
```

## エラーハンドリング

| 事象 | 挙動 |
|---|---|
| `gh` 未認証 | `gh auth login` を促すメッセージを表示して停止 |
| 対象タグなし | エラーメッセージで停止 |
| タグはあるが Release 未作成（CI 進行中） | run URL を提示。`gh run watch` で待機 → `gh release edit` を提案、または `gh release create` で先行作成を選択肢として提示 |
| 対象範囲にユーザー向けコミット 0 件 | 「ユーザー向けの変更がありません」と通知して停止（空更新しない） |
| `.96kudye/tmp/` への書き込みブロック | `save-temp-file` スキルへ誘導 |
