# Mairu

Wails（Go + React）で構築するGmail整理デスクトップアプリです。製品ビジョンやアーキテクチャ、機能ロードマップは [`docs/gmail_cleaner_design_v2.md`](docs/gmail_cleaner_design_v2.md) にまとめています。

実装の着手順と段階別の作業項目は [`docs/TASKS.md`](docs/TASKS.md) を参照してください。
実装時に使う issue 単位のバックログは [`docs/ISSUES.md`](docs/ISSUES.md) を参照してください。

## 現在の状態
- ✅ `gmail_cleaner_design_v2.docx` をMarkdownへ取り込み済み。
- ⏳ ソースコードは未生成。次のステップは設計ドキュメントの構成に沿ってWails v2プロジェクトを初期化すること。

## 必要環境（開発開始時）
- Go 1.22+
- Node.js 20+（Wails Reactテンプレートが要求するバージョン）
- Wails CLI（`go install github.com/wailsapp/wails/v2/cmd/wails@latest`）
- pnpm もしくは npm（フロントエンド依存管理）

## 推奨ディレクトリ構成
```
mairu/
├── frontend/         # React + Tailwindアプリ（Wailsテンプレート）
├── internal/         # Goパッケージ（gmail, claude, db, auth, ...）
├── app.go            # WailsがUIに公開するGoメソッド
├── main.go           # Wailsエントリーポイント
└── docs/             # 設計・開発ドキュメント
```

## 次のアクション
1. リポジトリ直下で `wails init`（React + Tailwindテンプレート）を実行。
2. 生成されたファイルを上記構成になるよう移動/リネーム。
3. 設計ドキュメント各章をGo/Reactモジュールとして具体化していく。

Codexとの協調作業手順などは [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md) を参照してください。
