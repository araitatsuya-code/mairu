# Repository Guidelines

## プロジェクト構成とモジュール配置
Mairu は Gmail 整理に特化した Wails (Go + React) デスクトップアプリです。リポジトリ直下には `main.go` と `app.go` を置き、ドメイン別ロジックは `internal/gmail` `internal/claude` `internal/db` `internal/auth` へ分納し、共通 DTO は `internal/types` に集約します。フロントエンドは `frontend/` 配下で、画面ごとのコンポーネントを `frontend/src/pages/<Feature>/` に整理してください。全体像とロードマップは `docs/gmail_cleaner_design_v2.md`、Codex 協調手順は `docs/DEVELOPMENT.md` を参照します。

## ビルド・テスト・開発コマンド
- `wails dev`（将来的には `make dev`）: Go と React を同時にホットリロード。
- `wails build`（`make build`）: 各プラットフォーム向け実行ファイルを生成。
- `go test ./...` + `go fmt ./...`: バックエンド検証と整形を一括実行。
- `pnpm install` → `pnpm lint && pnpm test`: フロントエンド依存解決と Lint/ユニットテスト。`npm` を使う場合も同じ順序で問題ありません。

## コーディング規約と命名
Go は `go fmt` を必ず通し、パッケージ名は単数形、公開シンボルは PascalCase、内部変数は camelCase を推奨します。React/TypeScript ではファイル拡張子を `.tsx` に統一し、コンポーネント名は PascalCase、フックやユーティリティは camelCase にします。スタイルは Tailwind CSS を前提に JSX 内でユーティリティクラスを適用し、画像や JSON などのアセットは各ページ配下へ配置してください。Wails 生成プロジェクトの ESLint/Prettier 設定が整い次第、その自動整形ルールに従います。

## 言語運用
- このワークスペースでは、ユーザーとのやり取りを原則として日本語で行います。
- コードコメント、ドキュメント、レビューコメントも、特別な理由がない限り日本語で記述します。
- 外部 API やライブラリに由来する英語識別子はそのまま使い、周辺説明を日本語で補います。

## テスト指針
Go 側は `<module>_test.go` に `TestFooBar` 形式でユニットテストを書き、Gmail 分類ロジックや Claude プロンプト生成など副作用の大きい処理は Table Driven Test でケース網羅を狙います。React は各コンポーネントの隣に `<Component>.test.tsx` を作成し、UI フローの手動確認が必要な場合は PR 説明に手順を記載します。PR 前には `go test ./...` と `pnpm test -- --runInBand` を実行し、失敗時は原因と再現方法をメモしてください。

## コミットと PR ガイドライン
履歴では `type: 要約`（例: `feat: Gmailクライアント骨組み追加`）の形式を使用します。コミット単位は「スキャフォールド追加」「機能実装」「ドキュメント更新」など論理的なまとまりを意識し、アーキテクチャや環境変数を変えた場合は関連ドキュメントも同時更新してください。PR では関連 Issue やタスク番号をリンクし、概要・詳細設計・実行したコマンド・テスト結果を箇条書きで示します。UI 変更がある場合はスクリーンショットや短い動画を添付し、レビュー担当者が同じ手順で検証できるよう説明を明確に保ちましょう。

## 推奨スキルセット
- Go 1.22 系のジェネリクスとコンテキスト API を扱えるレベル（Wails での非同期呼び出しを安定させるため）。
- React + TypeScript + Tailwind の実務経験、コンポーネント分割と状態管理（React Query や Zustand 想定）が得意だと移行が速いです。
- Gmail API / OAuth2 の理解、トークン更新やスコープ設計を CLI で検証できるスキル。
- Claude など LLM API のプロンプト設計・レート制御に慣れていること。
- `rg` `make` `pnpm` など CLI ツールの素早い操作と、ドキュメント整備の習慣。これらを備えていると初期フェーズから自走できます。
