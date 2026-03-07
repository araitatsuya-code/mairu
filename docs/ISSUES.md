# Mairu Issue バックログ

`docs/TASKS.md` を、実装時にそのまま使える issue 単位へ分割した一覧です。GitHub 上の Issue をまだ作成していない段階でも、このファイルのローカル ID を使って作業を進めます。

## 運用ルール
- 着手前に、必ずこのファイルから対象 issue を 1 つ選ぶ。
- ブランチ名、コミットメッセージ、PR 説明にはローカル ID（例: `MAIRU-004`）を含める。
- GitHub Issue を作る場合は、タイトル先頭にもローカル ID を残す（例: `[MAIRU-004] OAuth PKCE フロー実装`）。
- GitHub Issue 番号が発行されたら、PR には `MAIRU-004 / #12` のように両方を書く。
- GitHub Issue を作成したら、このファイルの一覧にも issue 番号を反映する。
- 着手したら GitHub Issue に `status: in progress` ラベルを付け、担当者を assignee に設定する。
- 完了したら `.github/ISSUE_CLOSE_COMMENT_TEMPLATE.md` を使ってコメントを残し、GitHub Issue の状態を `status: done` に更新してクローズする。
- 1つの変更で複数 issue にまたがる場合でも、主担当 issue を 1 つ決めてから着手する。

## ステータス定義
- `ready`: 着手可能
- `in progress`: 作業中
- `blocked`: 依存 issue 待ち
- `backlog`: まだ着手しない
- `done`: 完了してクローズ済み

## 優先順サマリー

| ローカル ID | GitHub | 状態 | フェーズ | 概要 | 依存 |
| --- | --- | --- | --- | --- | --- |
| MAIRU-001 | #1 | done | 準備 | Wails 初期化と開発コマンド整備 | なし |
| MAIRU-002 | #2 | done | 準備 | Go パッケージ骨組みと共有 DTO の作成 | MAIRU-001 |
| MAIRU-003 | #3 | done | Phase 1 | Settings 画面の雛形と初期化フロー | MAIRU-001 |
| MAIRU-004 | #4 | done | Phase 1 | Google OAuth PKCE ログイン実装 | MAIRU-001, MAIRU-003 |
| MAIRU-005 | #5 | done | Phase 1 | キーチェーン連携と機密情報保護 | MAIRU-004 |
| MAIRU-006 | #6 | done | Phase 1 | Gmail API クライアント初期化と接続確認 | MAIRU-004, MAIRU-005 |
| MAIRU-007 | #7 | done | Phase 2 | Claude API クライアントと分類 DTO | MAIRU-002, MAIRU-005 |
| MAIRU-008 | #8 | done | Phase 2 | 分類確認画面と信頼度分岐 UI | MAIRU-003, MAIRU-007 |
| MAIRU-009 | #9 | ready | Phase 2 | Gmail アクション実行とラベル管理 | MAIRU-006, MAIRU-008 |
| MAIRU-010 | #10 | done | Phase 3 | SQLite 初期化、スキーマ、マイグレーション | MAIRU-002 |
| MAIRU-011 | #11 | blocked | Phase 3 | ブロックリスト管理と AI スキップ処理 | MAIRU-009, MAIRU-010 |
| MAIRU-012 | #12 | blocked | Phase 3 | エクスポート機能と mbox 調査 | MAIRU-010, MAIRU-011 |
| MAIRU-013 | #13 | blocked | Phase 4 | 定期実行スケジューラーと再試行制御 | MAIRU-009, MAIRU-010, MAIRU-011 |
| MAIRU-014 | #14 | blocked | Phase 4 | OS 通知と自動実行設定 UI | MAIRU-003, MAIRU-013 |
| MAIRU-015 | 未作成 | backlog | Phase 5 | 移行アシスタント | MAIRU-012 |
| MAIRU-016 | 未作成 | backlog | Phase 6 | GitHub Actions リリース自動化 | MAIRU-001 |
| MAIRU-017 | 未作成 | backlog | v2+ | AI アシスタント機能 | MAIRU-014 |

## Issue 詳細

### MAIRU-001: Wails 初期化と開発コマンド整備
- 状態: `done`
- フェーズ: 準備
- GitHub: `#1`
- 目的: 開発に必要な最小構成を作り、ローカルで起動・ビルド・テストの入口をそろえる。
- 対応内容:
  - `wails init` による初期プロジェクト生成
  - `main.go` `app.go` `frontend/` `build/` の基本構成整理
  - `npm` によるフロントエンド依存管理の初期化
  - `Makefile` に `make dev` `make build` `make test` を定義
- 完了条件:
  - `wails dev` が起動する
  - フロントエンドと Go の雛形が同居した状態になる
  - 基本コマンドの入口が README または Makefile から分かる

### MAIRU-002: Go パッケージ骨組みと共有 DTO の作成
- 状態: `done`
- フェーズ: 準備
- 依存: `MAIRU-001`
- 目的: バックエンドの実装先を先に固定し、後続 issue で迷わないようにする。
- 対応内容:
  - `internal/gmail` `internal/claude` `internal/db` `internal/auth` `internal/types` を作成
  - 最低限のパッケージコメントとプレースホルダ型を定義
  - 共通 DTO の配置方針を `internal/types` に寄せる
- 完了条件:
  - 後続 issue が新規ディレクトリ作成なしで着手できる
  - 共有 DTO の置き場が明確になる

### MAIRU-003: Settings 画面の雛形と初期化フロー
- 状態: `done`
- フェーズ: Phase 1
- 依存: `MAIRU-001`
- 目的: 認証状態、設定不足、起動時の初期状態を UI から扱えるようにする。
- 対応内容:
  - `frontend/src/pages/Settings` の雛形作成
  - 起動時初期化処理の入口作成
  - 認証状態、API キー状態、通知設定などの表示領域を確保
- 完了条件:
  - 設定画面から初期状態が確認できる
  - 後続の OAuth / API キー実装を UI に接続できる

### MAIRU-004: Google OAuth PKCE ログイン実装
- 状態: `done`
- フェーズ: Phase 1
- 依存: `MAIRU-001`, `MAIRU-003`
- 目的: Google ログインから認可コード取得までのフローをローカルアプリとして成立させる。
- 対応内容:
  - PKCE 生成
  - localhost リダイレクト受信
  - 必要スコープの定義
  - 認証失敗時の再ログイン導線
- 完了条件:
  - ユーザーがアプリから Google ログインを開始できる
  - 認可コードを受け取りトークン交換へ進める

### MAIRU-005: キーチェーン連携と機密情報保護
- 状態: `done`
- フェーズ: Phase 1
- 依存: `MAIRU-004`
- 目的: OAuth トークンと Claude API キーを安全に保存し、SQLite やログへ漏らさないようにする。
- 対応内容:
  - OS キーチェーン連携ライブラリ選定
  - OAuth トークン保存 / 読み出し
  - Claude API キー保存 / 読み出し
  - ログ出力マスキング
- 完了条件:
  - 機密情報が DB や平文ファイルに保存されない
  - 再起動後も保存済み情報を再利用できる

### MAIRU-006: Gmail API クライアント初期化と接続確認
- 状態: `done`
- フェーズ: Phase 1
- 依存: `MAIRU-004`, `MAIRU-005`
- 目的: 認証済み状態で Gmail API を呼び、接続確認を取れるようにする。
- 対応内容:
  - Gmail API クライアント生成
  - トークン再利用
  - ユーザー情報または少量メール取得による接続確認
- 完了条件:
  - 再起動後も Gmail API に接続できる
  - Phase 1 の完了判定に使える接続確認が実装される

### MAIRU-007: Claude API クライアントと分類 DTO
- 状態: `done`
- フェーズ: Phase 2
- 依存: `MAIRU-002`, `MAIRU-005`
- 目的: メール分類ロジックの核となる Claude 連携とデータ構造を先に整える。
- 対応内容:
  - Claude API クライアント実装
  - モデル設定の切り替え
  - 分類カテゴリ DTO
  - 50 件バッチを前提にしたリクエスト / レスポンス整形
- 完了条件:
  - 分類 API を単体で呼び出せる
  - UI 側に渡す分類結果の型が固まる

### MAIRU-008: 分類確認画面と信頼度分岐 UI
- 状態: `done`
- フェーズ: Phase 2
- 依存: `MAIRU-003`, `MAIRU-007`
- 目的: 分類結果を見て承認できる UI を先に成立させる。
- 対応内容:
  - `frontend/src/pages/Classify` 実装
  - 信頼度表示
  - 自動実行、承認待ち、要確認の見せ分け
  - 推奨アクション表示
- 完了条件:
  - 50 件分の分類結果を一覧で確認できる
  - ユーザーが承認対象を判断できる

### MAIRU-009: Gmail アクション実行とラベル管理
- 状態: `ready`
- フェーズ: Phase 2
- 依存: `MAIRU-006`, `MAIRU-008`
- 目的: 承認済みの分類結果を実際の Gmail 操作へつなぐ。
- 対応内容:
  - ラベル作成 / 取得 / 適用
  - アーカイブ / 削除 / 既読化
  - 確認ステップを経た一括操作
- 完了条件:
  - 承認後に Gmail 側の状態が変わる
  - ラベル付与と削除が最低限機能する

### MAIRU-010: SQLite 初期化、スキーマ、マイグレーション
- 状態: `done`
- フェーズ: Phase 3
- 依存: `MAIRU-002`
- 目的: ブロックリスト、ログ、設定の永続化基盤を整える。
- 対応内容:
  - SQLite 接続初期化
  - WAL モード設定
  - `blocklist` `action_logs` `settings` テーブル作成
  - マイグレーション方針の確立
- 完了条件:
  - アプリ再起動後もデータが残る
  - スキーマ更新を安全に適用できる

### MAIRU-011: ブロックリスト管理と AI スキップ処理
- 状態: `blocked`
- フェーズ: Phase 3
- 依存: `MAIRU-009`, `MAIRU-010`
- 目的: コスト最適化のため、既知の不要送信者を AI 分析前に処理できるようにする。
- 対応内容:
  - `frontend/src/pages/Blocklist` 実装
  - 送信者 / ドメイン単位の登録
  - 修正履歴からの追加提案
  - ヒット時の AI スキップ処理
- 完了条件:
  - ブロックリストを UI から操作できる
  - 対象送信者で分類 API を呼ばずに処理できる

### MAIRU-012: エクスポート機能と mbox 調査
- 状態: `blocked`
- フェーズ: Phase 3
- 依存: `MAIRU-010`, `MAIRU-011`
- 目的: 処理結果や重要情報を外部へ持ち出せるようにする。
- 対応内容:
  - 処理済みメール一覧の CSV / JSON 出力
  - ブロックリストの JSON 出力 / 取込
  - 重要メールサマリーの CSV / PDF 出力
  - 日別ログの CSV / JSON 出力
  - `mbox` 候補ライブラリの調査メモ作成
- 完了条件:
  - 設計書の主要エクスポート（`mbox` 除く）が実行できる
  - `mbox` は次フェーズで着手できる調査結果が残る

### MAIRU-013: 定期実行スケジューラーと再試行制御
- 状態: `blocked`
- フェーズ: Phase 4
- 依存: `MAIRU-009`, `MAIRU-010`, `MAIRU-011`
- 目的: 起動中アプリで定期的に整理処理を動かす。
- 対応内容:
  - goroutine ベースのスケジューラー
  - 1 日 1 回の分類ジョブ
  - 1 日 1 回のブロック更新
  - 30 分ごとの既知ブロック処理
  - 最大 3 回の自動リトライ
  - 指数バックオフ
- 完了条件:
  - 設定どおりにジョブが回る
  - 二重実行や無限リトライが起きない

### MAIRU-014: OS 通知と自動実行設定 UI
- 状態: `blocked`
- フェーズ: Phase 4
- 依存: `MAIRU-003`, `MAIRU-013`
- 目的: 自動実行結果をユーザーに伝え、スケジュールを設定できるようにする。
- 対応内容:
  - OS 標準通知の実装
  - 完了件数、失敗件数、承認待ち件数の通知
  - `Settings` 画面の通知 ON/OFF
  - スケジュール設定 UI
- 完了条件:
  - 定期実行結果が通知される
  - ユーザーが設定画面から自動実行条件を変更できる

### MAIRU-015: 移行アシスタント
- 状態: `backlog`
- フェーズ: Phase 5
- 依存: `MAIRU-012`
- 目的: アカウント移行時のチェックリストと重要メール持ち出しを支援する。
- 対応内容:
  - `frontend/src/pages/Migration` 実装
  - サービス洗い出し
  - 重要度分類
  - 進捗管理
  - 旧アカウント後処理ガイド
- 完了条件:
  - チェックリストを生成し、保存・再開できる
  - 重要メールの持ち出し導線が明確になる

### MAIRU-016: GitHub Actions リリース自動化
- 状態: `backlog`
- フェーズ: Phase 6
- 依存: `MAIRU-001`
- 目的: タグプッシュで 3 OS 向け成果物を自動配布できるようにする。
- 対応内容:
  - リリースワークフロー追加
  - OS ごとのビルド確認
  - GitHub Releases 連携
  - 配布手順のドキュメント化
- 完了条件:
  - タグプッシュで配布物が生成される
  - リリース手順がドキュメントだけで追える

### MAIRU-017: AI アシスタント機能（v2+）
- 状態: `backlog`
- フェーズ: v2+
- 依存: `MAIRU-014`
- 目的: コア機能完了後に、自然言語で Gmail 整理を支援するチャット UI を追加する。
- 対応内容:
  - フローティングチャット UI
  - 会話履歴保存
  - JSON ベースのアクション提案
  - キャラクター定義と切り替え
- 完了条件:
  - コア機能を壊さず拡張できる設計が固まる
  - 削除系操作に必ず確認導線が入る

## GitHub Issue 化するときの手順
1. このファイルから対象 issue を選ぶ。
2. `.github/ISSUE_TEMPLATE/task-from-backlog.md` を使って GitHub Issue を作る。
3. GitHub で採番された番号を PR と作業メモに追記する。
4. 実装中は `docs/TASKS.md` ではなく、この issue 単位で進捗を管理する。

## 完了時のクローズ手順
1. 対応内容と完了条件を満たしていることを確認する。
2. `.github/ISSUE_CLOSE_COMMENT_TEMPLATE.md` を使って、対応内容・検証結果・関連 PR を issue にコメントする。
3. GitHub Issue から `status: in progress` など着手中のラベルを外す。
4. GitHub Issue に `status: done` を付ける。
5. PR や検証結果を issue に紐付けたうえで issue をクローズする。
6. 必要ならこのファイルの状態も `done` または完了済みとして更新する。
