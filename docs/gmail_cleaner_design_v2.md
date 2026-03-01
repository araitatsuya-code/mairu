
# Mairu 機能設計ドキュメント

- バージョン: v2.0 (Wails + React + Go)
- 更新日: 2026年2月

## 1. プロジェクト概要
MairuはClaude APIとGmail APIを組み合わせてGmail整理を自動化するデスクトップアプリです。Wails（Go + React）で実装し、ユーザーPC上で全処理を完結させることでサーバー不要・OAuth審査不要を実現します。

### 1.1 目的
- 溜まったメールを自動分類し、不要なものを削除・アーカイブする。
- 重要メールを見落とさないようラベル整理する。
- ブロックリストでメルマガ/不要送信者を継続的にブロックする。
- アカウント移行時の手間を減らす移行アシスタントを提供する。

### 1.2 全体アーキテクチャ
Gmail APIでメール取得 → Claude APIで分類 → アクション実行（削除/ラベル/アーカイブ）→ ブロックリスト更新。すべてローカルPC内で完結。

## 2. 機能一覧

### 2.1 メール分類エンジン（AI分析）
| カテゴリ | 対象メール | 実行アクション |
| --- | --- | --- |
| 🚨 重要 | 仕事・公式連絡・金融系 | ラベル付け・受信トレイ保持 |
| 📰 メルマガ | ニュースレター・プロモーション | 削除 or アーカイブ |
| 🗑️ 不要 | スパム・通知系メール | 即削除 |
| 📁 保存 | 領収書・契約書など | ラベル付けしてアーカイブ |
| ❓ 未読優先 | 未読の重要そうなメール | 優先ラベルを付与 |

### 2.2 ブロックリスト管理
- 1日1回監視してメルマガ・不要送信者を自動検出。
- 検出結果をSQLiteのブロックリストに蓄積。
- 次回以降はAI分析をスキップし即処理（Claude APIコスト最適化）。
- 手動での追加・除外・編集も可能。

### 2.3 エクスポート機能
| エクスポート対象 | 形式 | 用途 |
| --- | --- | --- |
| 処理済みメール一覧 | CSV / JSON | 削除・整理した履歴の記録 |
| ブロックリスト | JSON | バックアップ・別環境への移行 |
| 重要メールのサマリー | CSV / PDF | 後から振り返り用 |
| 分類ログ（日別） | JSON / CSV | AIの判断履歴の確認 |
| メール本体 | mbox ※ | 他メールクライアントへの移行 |

> ※ mboxはGo標準ライブラリにないためサードパーティ or 自前実装が必要。工数が大きいためPhase 5以降。

### 2.4 移行アシスタント
- 登録サービスの自動洗い出しと重要度分類。
- 移行チェックリストの生成・進捗管理。
- 重要メールのmbox形式エクスポート/インポート。
- 旧アカウント後処理ガイドとリマインダー。

## 3. デスクトップアプリ配布方針
本ツールはWails製デスクトップアプリとして配布し、サーバー/Google OAuth審査を不要にします。

### 3.1 配布方式のメリット
| 項目 | Webサービス | デスクトップアプリ |
| --- | --- | --- |
| サーバー | 常時稼働が必要 | 不要 |
| Google OAuth審査 | 一般公開には必須 | 不要（ローカルアプリ扱い） |
| ユーザーデータ管理 | サーバー保管 | ユーザーPC上のみ |
| Claude APIコスト | 開発者が負担 | ユーザーが自分のキーを使用 |
| 配布方法 | URL共有 | インストーラー配布 |

### 3.2 配布ファイル
| OS | 形式 | 備考 |
| --- | --- | --- |
| macOS | .dmg | Apple Silicon / Intel 両対応 |
| Windows | .exe（NSIS） | インストーラー形式 |
| Linux | .AppImage | 主要ディストリ対応 |

### 3.3 配布フロー
- GitHub Releasesにビルド済みファイルをアップロード。
- GitHub Actionsでタグプッシュ時に自動ビルド・リリース。
- ユーザーはリリースページからダウンロードしてインストール。

## 4. 技術スタック

### 4.1 レイヤー構成
| レイヤー | 技術 | 役割 |
| --- | --- | --- |
| フレームワーク | Wails v2 | GoとReactを繋ぐデスクトップ基盤 |
| バックエンド | Go 1.22+ | Gmail API・Claude API・ローカルDB操作 |
| フロントエンド | React + TypeScript | UI/画面操作 |
| スタイリング | Tailwind CSS | デザイン |
| ローカルDB | SQLite (modernc/sqlite, WAL) | ブロックリスト・処理ログ |
| Gmail操作 | google-api-go-client | メール取得/削除/ラベル操作 |
| AI分析 | Claude API (デフォルト: claude-sonnet-4-5-20250929) | メール分類 |
| 認証 | OAuth 2.0 + PKCE | Googleアカウント連携 |
| ビルド/配布 | GitHub Actions + Wails CLI | 自動ビルド・リリース |

### 4.2 プロジェクト構成
```
gmail-cleaner/
├── main.go                  # Wailsエントリーポイント
├── app.go                   # フロントから呼べるGoメソッド
├── internal/
│   ├── gmail/               # Gmail APIクライアント
│   │   ├── client.go
│   │   ├── fetch.go         # メール取得
│   │   └── actions.go       # 削除・ラベル・アーカイブ
│   ├── claude/              # Claude APIクライアント
│   │   ├── client.go
│   │   └── classify.go      # メール分類ロジック
│   ├── db/                  # SQLite操作
│   │   ├── db.go
│   │   ├── blocklist.go
│   │   └── logs.go
│   └── auth/                # OAuth 2.0
│       └── oauth.go
├── frontend/
│   ├── src/
│   │   ├── App.tsx
│   │   ├── pages/
│   │   │   ├── Classify.tsx      # メール分類・承認
│   │   │   ├── Blocklist.tsx     # ブロックリスト管理
│   │   │   ├── Export.tsx        # エクスポート
│   │   │   ├── Migration.tsx     # 移行アシスタント
│   │   │   └── Settings.tsx      # APIキー/設定
│   │   └── components/           # 共通コンポーネント
│   └── package.json
├── build/                   # ビルド設定・アイコン
└── .github/workflows/       # GitHub Actions
```

### 4.3 GoとReactの連携
WailsではGoのメソッドをそのままフロントから呼び出せる。API通信コードは不要で型安全なブリッジを自動生成。

```go
// app.go
func (a *App) FetchEmails(maxResults int) ([]Email, error) {
    return a.gmailClient.Fetch(maxResults)
}

func (a *App) ExecuteActions(actions []Action) error {
    return a.gmailClient.Execute(actions)
}
```

```tsx
// Classify.tsx
import { FetchEmails, ExecuteActions } from '../wailsjs/go/main/App'

const emails = await FetchEmails(100)
await ExecuteActions(selectedActions)
```

## 5. OAuth 2.0 認証フロー
ローカルポートリダイレクト方式を採用し、ブラウザでGoogleログイン→localhostでコード受領→トークン保存の流れ。

### 5.1 フロー
- アプリ起動時にlocalhost:PORTで一時サーバーを起動。
- Google OAuth URLを既定ブラウザで開く。
- ユーザーがログインして許可。
- localhostへリダイレクトされコード受領。
- アクセストークン/リフレッシュトークンをローカル保存。
- 以降は保存済みトークンで自動認証。

### 5.2 Google Cloud Console 設定
- プロジェクト作成→Gmail APIを有効化。
- OAuthクライアントIDを「デスクトップアプリ」で作成。
- Client IDをビルド設定に同梱。
- Client Secretは保持せずPKCEを採用（Google推奨）。

### 5.3 必要スコープ
| スコープ | 用途 |
| --- | --- |
| gmail.modify | メールの読み取り・ラベル付け・アーカイブ |
| gmail.labels | ラベルの作成・管理 |
| gmail.metadata | メタデータのみの取得（高速化） |

## 6. Claude API 連携

### 6.1 APIキー管理
| データ種別 | 保存先 | 理由 |
| --- | --- | --- |
| Claude APIキー | OSキーチェーン | 平文保存を避けるため |
| OAuthトークン | OSキーチェーン | 認証情報保護 |
| スケジュール/設定 | SQLite settingsテーブル | 機密性が低い |

ユーザーが設定画面でキーを入力し、SQLiteには保存しない。

### 6.2 分類プロンプト設計
- 本文全てではなく送信者・件名・冒頭200文字のみを送信しコスト削減。
- 最大50件のメールをバッチ処理。

```
分類カテゴリ:
  - important
  - newsletter
  - junk
  - archive
  - unread_priority

メール情報: [{ id, from, subject, snippet }]
レスポンス: [{ id, category, confidence, reason }]
```

### 6.3 信頼度別処理
| confidence | 動作 |
| --- | --- |
| 90%以上 | 自動実行キューへ（確認不要） |
| 70〜89% | 確認画面でユーザー承認後に実行 |
| 50〜69% | 確認画面に理由を補足表示 |
| 50%未満 | 自動実行しない。「要確認」ラベルで保留 |

### 6.4 フィードバックループ
- ユーザー訂正（例: 重要→不要）を`action_logs`に記録。
- Few-shot事例として蓄積しプロンプトを改善。
- 重要と判定したが不要だったケースも記録。
- 同一送信者への訂正が3回以上でブロックリスト追加を提案。

### 6.5 コスト目安
| 処理件数 | トークン概算 | Claude Sonnet料金目安 |
| --- | --- | --- |
| 50件/回 | 約3,000 tokens | 約$0.01 |
| 200件/日 | 約12,000 tokens | 約$0.04 |
| 月間運用 | 約360,000 tokens | 約$1.2 |

> ブロックリスト送信者は分析をスキップするため実コストはさらに低い。

## 7. ローカルDB設計（SQLite）

### 7.1 テーブル定義
**blocklist**
| カラム | 型 | 説明 |
| --- | --- | --- |
| id | INTEGER PK |  |
| sender_email | TEXT UNIQUE | ブロック対象メールアドレス |
| sender_domain | TEXT | ドメイン単位ブロック |
| reason | TEXT | ブロック理由 |
| auto_detected | BOOLEAN | AI自動検出か |
| created_at | DATETIME | 登録日時 |

**action_logs**
| カラム | 型 | 説明 |
| --- | --- | --- |
| id | INTEGER PK |  |
| message_id | TEXT | GmailメッセージID |
| from | TEXT | 送信者 |
| subject | TEXT | 件名 |
| action | TEXT | delete / archive / label |
| category | TEXT | AI分類結果 |
| confidence | REAL | 信頼度(0〜1) |
| status | TEXT | pending / success / failed |
| executed_at | DATETIME | 実行日時 |

**settings**
| カラム | 型 | 説明 |
| --- | --- | --- |
| key | TEXT PK | 設定キー |
| value | TEXT | 設定値（APIキー/OAuthトークン除く） |
| updated_at | DATETIME | 最終更新 |

## 8. エラーハンドリング・リトライ

### 8.1 レートリミット
| API | 制限 | 対策 |
| --- | --- | --- |
| Gmail API | 250 quota units/秒 | バッチ/指数バックオフでリトライ |
| Claude API | モデルごとにTPM/RPM | バッチサイズ調整・429時に待機 |

### 8.2 バッチ処理の途中再開
- 処理開始時に対象メールを`status=pending`で登録。
- 成功時に`status=success`に更新。
- 失敗は`status=failed`でエラー内容を書き戻し。
- 再実行時は`status=pending`のみ再開。

### 8.3 ユーザー通知
- 一時ネットワークエラー: 最大3回まで自動リトライしサイレント処理。
- 認証エラー: 再ログイン通知。
- APIキー不正: 設定画面への誘導。
- 一部失敗: 完了通知にスキップ件数を含める。

## 9. 定期自動実行
アプリ起動中はGo goroutineでスケジューラーを動かし、バックグラウンド処理を実施。

### 9.1 スケジュール
| タスク | 頻度 | 内容 |
| --- | --- | --- |
| メール分類 | 1日1回（設定可能） | 新着メールをAI分類に追加 |
| ブロックリスト更新 | 1日1回 | パターン検出して自動登録 |
| 既知ブロック処理 | 30分ごと | ブロック済み送信者を即処理 |

### 9.2 通知
- 処理完了時にOS標準通知（macOS: NSNotification, Windows: Toast）。
- 「84件を整理しました」などのサマリー表示。
- 承認が必要な重要メールが残る場合は別途通知。

## 10. GitHub Actions 自動ビルド/配布

### 10.1 フロー
mainブランチへのタグプッシュをトリガーにmacOS/Windows/Linux向けバイナリを自動生成しGitHub Releasesへアップロード。

### 10.2 ワークフロー例
```yaml
# .github/workflows/release.yml
on:
  push:
    tags: ['v*']

jobs:
  build:
    strategy:
      matrix:
        os: [macos-latest, windows-latest, ubuntu-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - run: go install github.com/wailsapp/wails/v2/cmd/wails@latest
      - run: wails build -platform ${{ matrix.os }}
      - uses: softprops/action-gh-release@v1
```

## 11. 開発フェーズ
| フェーズ | 内容 | 優先度 |
| --- | --- | --- |
| Phase 1 | Wailsプロジェクト作成 + OAuth認証 + Gmail API接続確認 | 🔴 最優先 |
| Phase 2 | Claude APIでメール分類 + 承認画面UI | 🔴 最優先 |
| Phase 3 | ブロックリスト管理 + SQLite + エクスポート機能 | 🟡 高 |
| Phase 4 | 定期自動実行 + OS通知 | 🟡 高 |
| Phase 5 | 移行アシスタント機能 | 🟢 中 |
| Phase 6 | GitHub Actionsの自動ビルド/配布 | 🟢 中 |

## 12. AIアシスタント機能（v2以降）
スコープが大きいためPhase 1〜4のコア機能完了後に実装。

### 12.1 UI仕様
| 要素 | 仕様 |
| --- | --- |
| 呼び出し方法 | 右下のフローティングボタン（キャラアイコン） |
| 表示形式 | 右下からスライドアップするチャットパネル（幅360px） |
| 常駐 | アプリ起動中は常にアクセス可能 |
| 会話履歴 | セッション継続中は保持し、SQLiteに保存して再利用 |

### 12.2 機能
1. **自然言語操作**: 指示を送るとAIが処理候補を提示し、承認後に実行。
   - 例: 「先週のメルマガ全部消して」→ newsletter分類を抽出。
   - 例: 「Amazonからのメールをアーカイブして」→ 送信元検索して候補提示。
2. **分類結果の説明**: 不要判定理由を説明。信頼度70%未満は詳細理由も提示。
3. **整理アドバイス**: 受信トレイ分析から優先順位やブロック提案、月次レポートを通知。
4. **移行サポート**: チャットから移行アシスタント進捗や手順を案内。

### 12.3 キャラクター選択
| キャラ名 | 雰囲気 | 口調例 |
| --- | --- | --- |
| Aria（デフォルト） | クールで頼れる秘書 | 「先週のメルマガ84件を削除候補としてリストアップしました。実行しますか？」 |
| Haru | フレンドリー | 「先週のメルマガ、84件見つけたよ！まとめて消しちゃう？」 |
| Kumo | おちゃめ | 「わあ、84件もメルマガ来てたんですね〜！全部やっつけますか？」 |
| Rex | 無口・効率重視 | 「対象: 84件 / 種別: newsletter / 実行しますか [y/n]」 |

キャラクターはJSONで定義し、将来のカスタム配布に対応。

```json
// characters/aria.json
{
  "id": "aria",
  "name": "Aria",
  "icon": "aria.png",
  "systemPrompt": "あなたはMairuのAIアシスタントAriaです。クールで簡潔、でも必要なときは丁寧に説明する有能な秘書として振る舞います。",
  "greetings": ["何かご指示はありますか？", "お手伝いできることはありますか？"]
}
```

### 12.4 技術構成
| 要素 | 技術・方針 |
| --- | --- |
| AIモデル | Claude API (claude-sonnet-4-6) |
| システムプロンプト | キャラJSONから動的生成 |
| 操作実行 | AIがJSON形式でアクション指示 → GoバックエンドがGmail APIを呼ぶ |
| 会話履歴 | SQLite (sessions/messages) |
| 安全確認 | 削除・移行などは必ず確認UIを挟む |

アクション指示フォーマット:
```json
{
  "action": "delete_emails",
  "filter": {
    "category": "newsletter",
    "date_from": "2026-02-17",
    "date_to": "2026-02-23"
  },
  "requires_confirmation": true,
  "preview_message": "先週のメルマガ84件を削除します。よろしいですか？"
}
```

## 13. 機能構成まとめ（最終版）
```
Mairu（Wails デスクトップアプリ）
├── Go バックエンド
│   ├── 分類エンジン（Claude API）
│   ├── Gmail APIクライアント（取得・削除・ラベル・アーカイブ）
│   ├── OAuth 2.0 認証（ローカルリダイレクト）
│   ├── ブロックリスト管理（SQLite）
│   ├── 定期自動実行（goroutineスケジューラー）
│   ├── エクスポート（CSV / JSON / PDF / mbox）
│   └── AIアシスタント実行エンジン
│
├── React フロントエンド
│   ├── メール分類・承認画面
│   ├── ブロックリスト管理UI
│   ├── エクスポート画面
│   ├── 移行アシスタント
│   ├── 設定画面（APIキー・スケジュール・キャラ選択）
│   └── AIアシスタントチャットUI（フローティング）
│       ├── キャラクター選択・表示
│       ├── 自然言語操作
│       └── 確認ダイアログ連携
│
├── キャラクター定義（JSON）
│   ├── aria.json
│   ├── haru.json
│   ├── kumo.json
│   └── rex.json
│
└── 配布
    ├── macOS (.dmg)
    ├── Windows (.exe)
    └── Linux (.AppImage)
```
