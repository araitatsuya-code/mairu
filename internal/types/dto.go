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

const (
	ClassificationMaxBatchSize      = 50
	ClassificationAutoApplyMinimum  = 0.90
	ClassificationReviewMinimum     = 0.70
	ClassificationReasonHintMinimum = 0.50
)

// IsValid は既知の分類カテゴリかを判定する。
func (c ClassificationCategory) IsValid() bool {
	switch c {
	case ClassificationCategoryImportant,
		ClassificationCategoryNewsletter,
		ClassificationCategoryJunk,
		ClassificationCategoryArchive,
		ClassificationCategoryUnreadPriority:
		return true
	default:
		return false
	}
}

// ClassificationReviewLevel は信頼度に応じた UI 上の扱いを表す。
type ClassificationReviewLevel string

const (
	ClassificationReviewLevelAutoApply        ClassificationReviewLevel = "auto_apply"
	ClassificationReviewLevelReview           ClassificationReviewLevel = "review"
	ClassificationReviewLevelReviewWithReason ClassificationReviewLevel = "review_with_reason"
	ClassificationReviewLevelHold             ClassificationReviewLevel = "hold"
)

// ReviewLevelForConfidence は信頼度から UI の分岐を決める。
func ReviewLevelForConfidence(confidence float64) ClassificationReviewLevel {
	switch {
	case confidence >= ClassificationAutoApplyMinimum:
		return ClassificationReviewLevelAutoApply
	case confidence >= ClassificationReviewMinimum:
		return ClassificationReviewLevelReview
	case confidence >= ClassificationReasonHintMinimum:
		return ClassificationReviewLevelReviewWithReason
	default:
		return ClassificationReviewLevelHold
	}
}

// ClassificationRequest は Claude 分類 API 呼び出し入力を表す。
type ClassificationRequest struct {
	Model    string
	Messages []EmailSummary
}

// ClassificationResult は分類 API と UI 間で共有する結果 DTO。
type ClassificationResult struct {
	MessageID   string
	Category    ClassificationCategory
	Confidence  float64
	Reason      string
	ReviewLevel ClassificationReviewLevel
}

// ClassificationResponse は Claude 分類 API 呼び出し結果を表す。
type ClassificationResponse struct {
	Model   string
	Results []ClassificationResult
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
	Authorized         bool
	GoogleConfigured   bool
	AuthStatus         string
	GoogleTokenPreview string
	GmailConnected     bool
	GmailStatus        string
	GmailAccountEmail  string
	ClaudeConfigured   bool
	ClaudeStatus       string
	ClaudeKeyPreview   string
	DatabaseReady      bool
	LastRunAt          *time.Time
}

// GoogleLoginResult は Google OAuth ログイン実行結果を表す。
type GoogleLoginResult struct {
	Success            bool
	Message            string
	AuthorizationURL   string
	RedirectURL        string
	TokenStored        bool
	RefreshTokenStored bool
	StoredPreview      string
	Scopes             []string
}

// SecretOperationResult は機密情報の保存・削除結果を表す。
type SecretOperationResult struct {
	Success bool
	Message string
}

// GmailConnectionResult は Gmail API の接続確認結果を表す。
type GmailConnectionResult struct {
	Success        bool
	Message        string
	EmailAddress   string
	MessagesTotal  int64
	ThreadsTotal   int64
	HistoryID      string
	TokenRefreshed bool
}
