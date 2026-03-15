package types

import (
	"net/mail"
	"strings"
	"time"
)

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

// IsValid は既知のレビュー分岐かを判定する。
func (l ClassificationReviewLevel) IsValid() bool {
	switch l {
	case ClassificationReviewLevelAutoApply,
		ClassificationReviewLevelReview,
		ClassificationReviewLevelReviewWithReason,
		ClassificationReviewLevelHold:
		return true
	default:
		return false
	}
}

// ClassificationSource は分類結果の生成元を表す。
type ClassificationSource string

const (
	ClassificationSourceClaude    ClassificationSource = "claude"
	ClassificationSourceBlocklist ClassificationSource = "blocklist"
)

// IsValid は既知の分類ソースかを判定する。
func (s ClassificationSource) IsValid() bool {
	switch s {
	case ClassificationSourceClaude, ClassificationSourceBlocklist:
		return true
	default:
		return false
	}
}

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
	Source      ClassificationSource
}

// ClassificationResponse は Claude 分類 API 呼び出し結果を表す。
type ClassificationResponse struct {
	Model   string
	Results []ClassificationResult
}

// FetchClassificationMessagesRequest は Classify 画面向け Gmail 取得条件を表す。
type FetchClassificationMessagesRequest struct {
	Query      string
	MaxResults int
	LabelIDs   []string
	PageToken  string
}

// FetchClassificationMessagesResult は Classify 画面向け Gmail 取得結果を表す。
type FetchClassificationMessagesResult struct {
	Messages       []EmailSummary
	NextPageToken  string
	TokenRefreshed bool
}

// GmailLabel は Gmail ラベルの最小情報を表す。
type GmailLabel struct {
	ID   string
	Name string
	Type string
}

// ListGmailLabelsResult は Gmail ラベル一覧取得結果を表す。
type ListGmailLabelsResult struct {
	Labels         []GmailLabel
	TokenRefreshed bool
}

// GmailMessageHeader はメールヘッダ 1 件を表す。
type GmailMessageHeader struct {
	Name  string
	Value string
}

// GmailMessageDetail は 1 通分の詳細情報を表す。
type GmailMessageDetail struct {
	ID       string
	ThreadID string
	From     string
	To       string
	Subject  string
	Date     string
	Snippet  string
	LabelIDs []string
	Unread   bool
	BodyText string
	BodyHTML string
	Headers  []GmailMessageHeader
}

// BlocklistKind はブロックリストの登録単位を表す。
type BlocklistKind string

const (
	BlocklistKindSender BlocklistKind = "sender"
	BlocklistKindDomain BlocklistKind = "domain"
)

// IsValid は既知のブロック種別かを判定する。
func (k BlocklistKind) IsValid() bool {
	switch k {
	case BlocklistKindSender, BlocklistKindDomain:
		return true
	default:
		return false
	}
}

// BlocklistEntry はブロックリスト 1 件分を表す。
type BlocklistEntry struct {
	ID        int64
	Kind      BlocklistKind
	Pattern   string
	Note      string
	CreatedAt string
	UpdatedAt string
}

// BlocklistSuggestion は修正履歴ベースの提案を表す。
type BlocklistSuggestion struct {
	Kind        BlocklistKind
	Pattern     string
	Count       int
	LastSeenAt  string
	Description string
}

// UpsertBlocklistEntryRequest はブロックリスト登録入力を表す。
type UpsertBlocklistEntryRequest struct {
	Kind    BlocklistKind
	Pattern string
	Note    string
}

// BlocklistOperationResult はブロックリスト操作の実行結果を表す。
type BlocklistOperationResult struct {
	Success bool
	Message string
}

// ClassificationCorrection は分類修正履歴の登録入力を表す。
type ClassificationCorrection struct {
	MessageID         string
	Sender            string
	OriginalCategory  ClassificationCategory
	CorrectedCategory ClassificationCategory
}

// NormalizeSenderAddress は送信者文字列から比較用メールアドレスを抽出する。
func NormalizeSenderAddress(raw string) string {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	if trimmed == "" {
		return ""
	}

	if parsed, err := mail.ParseAddress(trimmed); err == nil {
		return strings.TrimSpace(strings.ToLower(parsed.Address))
	}

	if strings.Count(trimmed, "@") == 1 && !strings.Contains(trimmed, " ") {
		return trimmed
	}

	if strings.Contains(trimmed, "<") && strings.Contains(trimmed, ">") {
		start := strings.Index(trimmed, "<")
		end := strings.LastIndex(trimmed, ">")
		if start >= 0 && end > start+1 {
			candidate := strings.TrimSpace(trimmed[start+1 : end])
			if strings.Count(candidate, "@") == 1 && !strings.Contains(candidate, " ") {
				return candidate
			}
		}
	}

	return ""
}

// SenderDomain は送信者からドメイン部を抽出する。
func SenderDomain(sender string) string {
	address := NormalizeSenderAddress(sender)
	at := strings.LastIndex(address, "@")
	if at < 0 || at+1 >= len(address) {
		return ""
	}
	return address[at+1:]
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

// GmailActionDecision は承認済み分類から導いた 1 通分の実行判断を表す。
type GmailActionDecision struct {
	MessageID   string
	Category    ClassificationCategory
	ReviewLevel ClassificationReviewLevel
}

// ExecuteGmailActionsRequest は Gmail アクション実行入力を表す。
type ExecuteGmailActionsRequest struct {
	Confirmed bool
	Decisions []GmailActionDecision
	Metadata  []GmailActionMetadata
}

// GmailActionFailure は Gmail アクション失敗詳細を表す。
type GmailActionFailure struct {
	MessageID string
	Action    ActionKind
	Error     string
}

// ExecuteGmailActionsResult は Gmail アクション実行結果を表す。
type ExecuteGmailActionsResult struct {
	Success         bool
	Message         string
	ProcessedCount  int
	SuccessCount    int
	FailureCount    int
	SkippedCount    int
	DeletedCount    int
	ArchivedCount   int
	MarkedReadCount int
	LabeledCount    int
	CreatedLabels   []string
	Failures        []GmailActionFailure
	TokenRefreshed  bool
}

// GmailActionMetadata はログ保存に必要なメール情報を表す。
type GmailActionMetadata struct {
	MessageID   string
	ThreadID    string
	From        string
	Subject     string
	Category    ClassificationCategory
	Confidence  float64
	ReviewLevel ClassificationReviewLevel
	Source      ClassificationSource
}

// OperationResult は一般的な実行結果を表す。
type OperationResult struct {
	Success bool
	Message string
}

// RecordClassificationRunRequest は分類結果一括保存の入力を表す。
type RecordClassificationRunRequest struct {
	Messages []EmailSummary
	Results  []ClassificationResult
}

// ClassificationLogEntry は分類ログ 1 件分を表す。
type ClassificationLogEntry struct {
	ID           int64
	MessageID    string
	ThreadID     string
	From         string
	Subject      string
	Snippet      string
	Category     ClassificationCategory
	Confidence   float64
	ReviewLevel  ClassificationReviewLevel
	Source       ClassificationSource
	ClassifiedAt string
}

// ActionLogEntry は処理済みメールログ 1 件分を表す。
type ActionLogEntry struct {
	ID          int64
	MessageID   string
	ThreadID    string
	From        string
	Subject     string
	ActionKind  ActionKind
	Status      string
	Detail      string
	Category    ClassificationCategory
	Confidence  float64
	ReviewLevel ClassificationReviewLevel
	Source      ClassificationSource
	CreatedAt   string
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
	GWSAvailable       bool
	GWSStatus          string
	DatabaseReady      bool
	LastRunAt          *time.Time
}

// GWSCLIErrorKind は gws 実行失敗時の分類を表す。
type GWSCLIErrorKind string

const (
	GWSCLIErrorKindNone           GWSCLIErrorKind = "none"
	GWSCLIErrorKindNotInstalled   GWSCLIErrorKind = "not_installed"
	GWSCLIErrorKindAuth           GWSCLIErrorKind = "auth"
	GWSCLIErrorKindInvalidCommand GWSCLIErrorKind = "invalid_command"
	GWSCLIErrorKindTimeout        GWSCLIErrorKind = "timeout"
	GWSCLIErrorKindExecution      GWSCLIErrorKind = "execution"
)

// GWSDiagnosticsResult は gws 導入診断の結果を表す。
type GWSDiagnosticsResult struct {
	Success    bool
	Available  bool
	Message    string
	BinaryPath string
	Version    string
	Command    string
	Output     string
	ErrorKind  GWSCLIErrorKind
}

// GWSGmailDryRunRequest は gws Gmail dry-run の入力を表す。
type GWSGmailDryRunRequest struct {
	Query      string
	MaxResults int
}

// GWSGmailDryRunResult は gws Gmail dry-run の結果を表す。
type GWSGmailDryRunResult struct {
	Success    bool
	Message    string
	BinaryPath string
	Command    string
	Output     string
	ErrorKind  GWSCLIErrorKind
}

// SchedulerSettings は定期実行と通知の設定を表す。
type SchedulerSettings struct {
	ClassificationIntervalMinutes int
	BlocklistIntervalMinutes      int
	KnownBlockIntervalMinutes     int
	NotificationsEnabled          bool
}

// UpdateSchedulerSettingsRequest は定期実行設定の更新入力を表す。
type UpdateSchedulerSettingsRequest struct {
	ClassificationIntervalMinutes int
	BlocklistIntervalMinutes      int
	KnownBlockIntervalMinutes     int
	NotificationsEnabled          bool
}

// SchedulerNotification はスケジューラー実行結果の通知ペイロードを表す。
type SchedulerNotification struct {
	Title string
	Body  string
	Level string
	JobID string
	At    string
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
