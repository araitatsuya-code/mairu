# 開発ガイド（Codex向け）

このリポジトリはCodex CLIエージェントでの作業を想定して調整しています。以下のフローに沿うと、再現性の高い進め方ができます。

## 1. タスク受領
1. コンテキスト確認
   - [`docs/gmail_cleaner_design_v2.md`](gmail_cleaner_design_v2.md) を読む。
   - 実装状況は `README.md` を参照。
   - 着手順は [`docs/TASKS.md`](TASKS.md) を参照。
2. ユーザー依頼には具体的なゴールを書いてもらう（例: 「Gmailクライアントの骨組みを作成」）。
3. 複数ファイルにまたがる作業は Codex CLI の `/plan` で計画を立て、完了ごとに更新。

## 2. ローカル環境
- Go 1.22+、Node.js 20+、Wails CLI をインストール。
- 推奨パッケージマネージャーは `pnpm`（なければ `npm`）。
- 今後用意する想定コマンド:
  - `make dev` → `wails dev` でライブリロード。
  - `make build` → `wails build` で各OSビルド。
  - `make test` → `go test ./...` + フロントエンドのユニットテスト。

`Makefile` が整うまでは各コマンドを直接実行してください。

## 3. ディレクトリ規約
- `app.go`: Wails経由でフロントエンドから呼べるGoメソッドを公開。
- `internal/gmail`, `internal/claude`, `internal/db`, `internal/auth`: ドメインごとの処理を配置。
- `frontend/src/pages/*`: Classify/Blocklist/Export/Migration/Settings等の画面に対応。
- 横断的な型（メールモデルや共有DTO）は `internal/types` を作成して集約する方針。

## 4. Codex CLIでの作業
- **短いコマンドを使う**: `rg`, `ls`, `sed` など軽いツールを優先。
- **ユーザー変更のリバート禁止**: 指示がある場合のみ。
- **ファイル参照は引用**: 回答では `path/to/file:42` 形式で示す。
- **テスト**: 実行できない場合は検証手順を文章で説明。
- **ネットワーク**: 制限があるため、必要時のみ許可を求める。

## 5. ブランチ・コミット方針
- 追加ブランチが必要になるまで `main` をデフォルトとする。
- PRポリシーに合わせて機能単位でSquash、そうでなければ論理的な変更ごとにコミット。
- 可能ならコミットメッセージに課題IDや内容を含める（例: `feat: Gmailクライアント骨組み追加`）。

## 6. タスク完了前の品質チェック
- [ ] `go fmt ./...` / `go test ./...`（コード生成後）。
- [ ] `pnpm lint` / `pnpm test`（設定後）。
- [ ] 新しい設定値や環境変数をドキュメントで説明。
- [ ] アーキテクチャ/ワークフロー変更時は `README` や `docs/` を更新。
- [ ] Codexの最終応答に検証ステップを記載。

## 7. 参考リンク
- [Wails Docs](https://wails.io/docs/introduction)
- [Google Gmail API for Go](https://pkg.go.dev/google.golang.org/api/gmail/v1)
- [Claude API](https://docs.anthropic.com/)

ツールやプロセスが変わったら本ガイドも更新して、次回以降のCodex作業がスムーズになるようにしてください。
