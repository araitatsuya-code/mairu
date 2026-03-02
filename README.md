# Mairu

Wails（Go + React）で構築するGmail整理デスクトップアプリです。製品ビジョンやアーキテクチャ、機能ロードマップは [`docs/gmail_cleaner_design_v2.md`](docs/gmail_cleaner_design_v2.md) にまとめています。

実装の着手順と段階別の作業項目は [`docs/TASKS.md`](docs/TASKS.md) を参照してください。
実装時に使う issue 単位のバックログは [`docs/ISSUES.md`](docs/ISSUES.md) を参照してください。

## 現在の状態
- ✅ `gmail_cleaner_design_v2.docx` をMarkdownへ取り込み済み。
- ✅ Wails v2 + React (TypeScript) の初期スキャフォールドを配置済み。
- ✅ `MAIRU-001 / #1` を完了し、Wails の起動・ビルド・テスト導線を整備済み。
- ✅ `MAIRU-002 / #2` を完了し、`internal/` の Go パッケージ骨組みと共有 DTO を追加済み。
- ✅ `MAIRU-003 / #3` と `MAIRU-010 / #10` が着手可能。

## 必要環境（開発開始時）
- Go 1.22+
- Node.js 20+（Wails Reactテンプレートが要求するバージョン）
- Wails CLI（`go install github.com/wailsapp/wails/v2/cmd/wails@latest`）
- npm（フロントエンド依存管理）

## 推奨ディレクトリ構成
```
mairu/
├── frontend/         # React + Viteアプリ（Wailsテンプレート）
├── internal/         # Goパッケージ（gmail, claude, db, auth, ...）
├── app.go            # WailsがUIに公開するGoメソッド
├── main.go           # Wailsエントリーポイント
└── docs/             # 設計・開発ドキュメント
```

## 開発コマンド
1. `make frontend-install`
2. `make dev`
3. `make build`
4. `make test`

`wails` が PATH にない場合も、`$HOME/go/bin/wails` があれば `Makefile` から利用します。
macOS では `UniformTypeIdentifiers` のリンク設定を `Makefile` 側で補っています。

## 次のアクション
1. `MAIRU-003 / #3` に進み、Settings 画面の雛形と初期化導線を整える。
2. `MAIRU-010 / #10` に進み、SQLite 初期化とスキーマの土台を作る。
3. 設計ドキュメント各章を Go / React モジュールとして具体化していく。

Codexとの協調作業手順などは [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md) を参照してください。
