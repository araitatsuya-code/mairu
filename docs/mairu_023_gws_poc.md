# MAIRU-023: Google Workspace CLI (`gws`) 統合 PoC

## 目的

MAIRU の既存 Gmail API 実装 (`internal/gmail`) を置き換えずに、将来の AI アシスタント機能向け拡張経路として `gws` 連携を検証する。

この PoC では以下のみを実施する。

- `gws --version` の実行診断
- `gws gmail users messages list --dry-run` の read-only プレビュー
- エラー分類（認証不備 / コマンド不正 / タイムアウト / 未導入 / 実行失敗）の UI 返却

## 導入手順

`gws` は **オプショナル依存**。未導入でも Mairu の既存機能は利用可能。

### 1. CLI のインストール

```bash
npm install -g @googleworkspace/cli
```

または Homebrew:

```bash
brew install googleworkspace-cli
```

### 2. 認証（必要に応じて）

```bash
gws auth setup
gws auth login
```

read-only dry-run は環境により認証なしで実行可能だが、実 API 呼び出し検証時は認証済みを推奨。

### 3. Mairu 設定画面での確認

`Settings` 画面の `Google Workspace CLI (gws) PoC` から:

1. `gws 診断を実行`（`--version`）
2. `Gmail dry-run 候補を取得`（`gmail users messages list --dry-run`）

## 実装ポイント

- Go ラッパー: `internal/gws/client.go`
- App API:
  - `CheckGWSDiagnostics()`
  - `PreviewGWSGmailDryRun(request)`
- UI:
  - `frontend/src/pages/Settings/SettingsPage.tsx`

## エラー分類ルール

`gws` の終了コード（README 記載）を優先して分類する。

- `2`: `auth`（認証不備）
- `3`: `invalid_command`（引数/コマンド不正）
- context deadline/cancel: `timeout`
- バイナリ未検出: `not_installed`
- その他: `execution`

## 制約と注意事項

- `gws` は 0.x 系で、破壊的変更が起きる可能性がある。
- `gws` は Google の公式サポート製品ではない（community/project ベース）。
- 本 PoC は `--dry-run` 前提であり、破壊的操作の本実行は対象外。
- 既存の `internal/gmail` 実装は引き続き正規経路として保持する。

## 今後の判断材料

- `gws` バージョン更新時の追従コスト
- 認証導線の安定性（ローカル/CI）
- dry-run 出力の機械可読性（UI の候補提示に必要な情報量）
