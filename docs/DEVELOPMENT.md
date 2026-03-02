# 開発ガイド（Codex向け）

このリポジトリはCodex CLIエージェントでの作業を想定して調整しています。以下のフローに沿うと、再現性の高い進め方ができます。

## 1. タスク受領
1. コンテキスト確認
   - [`docs/gmail_cleaner_design_v2.md`](gmail_cleaner_design_v2.md) を読む。
   - 実装状況は `README.md` を参照。
   - 着手順は [`docs/TASKS.md`](TASKS.md) を参照。
   - 実際に着手する issue は [`docs/ISSUES.md`](ISSUES.md) から選ぶ。
2. ユーザー依頼には具体的なゴールを書いてもらう（例: 「Gmailクライアントの骨組みを作成」）。
3. まず 1 つの issue を主担当として決め、依存関係を確認してから着手する。
4. 複数ファイルにまたがる作業は Codex CLI の `/plan` で計画を立て、完了ごとに更新。
5. 着手した issue は GitHub 側で `status: in progress` にし、担当者を assignee に設定する。
6. 完了した issue は `.github/ISSUE_CLOSE_COMMENT_TEMPLATE.md` で完了コメントを残し、`status: done` に更新してからクローズする。

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
- 作業時は `docs/ISSUES.md` のローカル ID を必ず紐付ける（例: `MAIRU-004`）。
- ブランチを切る場合は `codex/mairu-004-oauth-pkce` のように issue ID を含める。
- コミットメッセージにも issue ID を含める（例: `feat: MAIRU-004 OAuth PKCE フロー追加`）。

## 6. Issue 駆動の進め方
- `docs/TASKS.md` は全体計画、`docs/ISSUES.md` は実装単位の backlog として扱う。
- 新しい作業を始める前に、対象 issue の依存が解消されているか確認する。
- GitHub で issue を作る場合は `.github/ISSUE_TEMPLATE/task-from-backlog.md` を使い、ローカル ID をタイトルに残す。
- 作業開始時は GitHub Issue に `status: in progress` を付け、開始前の状態ラベルを外す。
- 作業完了時は `.github/ISSUE_CLOSE_COMMENT_TEMPLATE.md` を使って完了コメントを残す。
- その後、GitHub Issue に `status: done` を付け、未完了ステータスを外してクローズする。
- PR にはローカル ID と GitHub issue 番号（あれば両方）を記載する。

## 7. タスク完了前の品質チェック
- [ ] `go fmt ./...` / `go test ./...`（コード生成後）。
- [ ] `pnpm lint` / `pnpm test`（設定後）。
- [ ] 新しい設定値や環境変数をドキュメントで説明。
- [ ] アーキテクチャ/ワークフロー変更時は `README` や `docs/` を更新。
- [ ] Codexの最終応答に検証ステップを記載。

## 8. 参考リンク
- [Wails Docs](https://wails.io/docs/introduction)
- [Google Gmail API for Go](https://pkg.go.dev/google.golang.org/api/gmail/v1)
- [Claude API](https://docs.anthropic.com/)

ツールやプロセスが変わったら本ガイドも更新して、次回以降のCodex作業がスムーズになるようにしてください。
