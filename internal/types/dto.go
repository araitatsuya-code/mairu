package types

import "time"

// EmailSummary は Gmail と Claude の間で共通利用するメール概要 DTO。
type EmailSummary struct {
	ID       string
	ThreadID string
	From     string
	Subject  string
	Snippet  string
	Unread   bool
}

// ClassificationCategory はメール分類カテゴリを表す。
type ClassificationCategory string

const (
	ClassificationCategoryImportant      ClassificationCategory = "important"
	ClassificationCategoryNewsletter     ClassificationCategory = "newsletter"
	ClassificationCategoryJunk           ClassificationCategory = "junk"
	ClassificationCategoryArchive        ClassificationCategory = "archive"
	ClassificationCategoryUnreadPriority ClassificationCategory = "unread_priority"
)

// ClassificationResult は分類 API と UI 間で共有する結果 DTO。
type ClassificationResult struct {
	MessageID  string
	Category   ClassificationCategory
	Confidence int
	Reason     string
}

// ActionKind は Gmail に対する実行種別を表す。
type ActionKind string

const (
	ActionKindLabel    ActionKind = "label"
	ActionKindArchive  ActionKind = "archive"
	ActionKindDelete   ActionKind = "delete"
	ActionKindMarkRead ActionKind = "mark_read"
)

// MessageAction は 1 通分の実行アクションを表す。
type MessageAction struct {
	MessageID string
	Kind      ActionKind
	Label     string
}

// RuntimeStatus は設定画面や初期化処理で共有する状態 DTO。
type RuntimeStatus struct {
	Authorized       bool
	GoogleConfigured bool
	AuthStatus       string
	ClaudeConfigured bool
	DatabaseReady    bool
	LastRunAt        *time.Time
}

// GoogleLoginResult は Google OAuth ログイン実行結果を表す。
type GoogleLoginResult struct {
	Success          bool
	Message          string
	AuthorizationURL string
	RedirectURL      string
	CodePreview      string
	Scopes           []string
}
