package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"mairu/internal/auth"
	"mairu/internal/claude"
	"mairu/internal/db"
	"mairu/internal/exporter"
	"mairu/internal/gmail"
	"mairu/internal/gws"
	"mairu/internal/scheduler"
	"mairu/internal/types"
)

const (
	gmailConnectionTimeout      = 15 * time.Second
	gmailActionTimeout          = 45 * time.Second
	claudeClassificationTimeout = 45 * time.Second
	gwsCommandTimeout           = 20 * time.Second
	dbOperationTimeout          = 10 * time.Second
	blocklistSuggestionMinimum  = 3
	defaultExportDirName        = "Downloads"

	defaultClassificationIntervalMinutes = int((24 * time.Hour) / time.Minute)
	defaultBlocklistUpdateMinutes        = int((24 * time.Hour) / time.Minute)
	defaultKnownBlockProcessMinutes      = int((30 * time.Minute) / time.Minute)
	minSchedulerIntervalMinutes          = 1
	maxSchedulerRetries                  = 3
	schedulerRetryBackoffBase            = 2 * time.Second

	schedulerSettingClassificationMinutes    = "scheduler.classification.interval_minutes"
	schedulerSettingBlocklistMinutes         = "scheduler.blocklist.interval_minutes"
	schedulerSettingKnownBlockMinutes        = "scheduler.known_block.interval_minutes"
	schedulerSettingNotificationsEnabled     = "scheduler.notifications.enabled"
	schedulerSettingLastRunAt                = "scheduler.last_run_at"
	schedulerSettingLastRunTemplate          = "scheduler.%s.last_run_at"
	schedulerSettingClassificationCheckpoint = "scheduler.classification.checkpoint"

	schedulerJobClassification = "classification_daily"
	schedulerJobBlocklist      = "blocklist_daily"
	schedulerJobKnownBlock     = "known_block_30m"

	schedulerCheckpointRunTypeScheduled = "scheduled"
	schedulerCheckpointModeSafeRun      = "safe-run"

	schedulerMessageMissingClassificationLastRun = "last_run_at 未設定のため、定期 safe-run は backlog 全量を自動実行しません。Classify 画面で手動実行して開始時刻を確定してください。"
	actionLogStatusPending                       = "pending"
	actionLogStatusSuccess                       = "success"
	actionLogStatusFailed                        = "failed"
	actionLogDetailDuplicateSkip                 = "重複防止: action_logs の最新 status=success のため Gmail 反映をスキップしました。"

	schedulerNotificationEventName = "scheduler:notification"
)

type classificationCheckpoint struct {
	RunType          string   `json:"run_type"`
	Mode             string   `json:"mode"`
	LastRunAt        string   `json:"last_run_at"`
	Query            string   `json:"query"`
	LabelIDs         []string `json:"label_ids"`
	CompletedBatches int      `json:"completed_batches"`
	Processed        int      `json:"processed"`
	Success          int      `json:"success"`
	Failed           int      `json:"failed"`
	PendingApproval  int      `json:"pending_approval"`
	Skipped          int      `json:"skipped"`
	LastStopReason   string   `json:"last_stop_reason,omitempty"`
	LastError        string   `json:"last_error,omitempty"`
	UpdatedAt        string   `json:"updated_at"`
}

type App struct {
	ctx           context.Context
	authClient    *auth.Client
	claudeClient  *claude.Client
	gmailClient   *gmail.Client
	gwsClient     *gws.Client
	secretManager *auth.SecretManager
	dbStore       *db.Store

	mu             sync.RWMutex
	authStatus     string
	gmailStatus    string
	claudeStatus   string
	gmailConnected bool
	gmailAccount   string
	databaseReady  bool
	loginCancel    context.CancelFunc
	loginCancelSeq uint64
	schedulerSvc   *scheduler.Service
	schedulerStop  context.CancelFunc
	eventsEmit     func(context.Context, string, ...interface{})

	runScheduledClassificationJob func(context.Context) (scheduler.Result, error)
	runScheduledBlocklistJob      func(context.Context) (scheduler.Result, error)
	runScheduledKnownBlockJob     func(context.Context) (scheduler.Result, error)
}

func NewApp() *App {
	clientID := strings.TrimSpace(os.Getenv("MAIRU_GOOGLE_OAUTH_CLIENT_ID"))
	clientSecret := strings.TrimSpace(os.Getenv("MAIRU_GOOGLE_OAUTH_CLIENT_SECRET"))
	claudeModel := strings.TrimSpace(os.Getenv("MAIRU_CLAUDE_MODEL"))
	secretManager := auth.NewSecretManager(auth.NewSystemSecretStore())

	app := &App{
		authClient: auth.NewClient(auth.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
		}),
		claudeClient: claude.NewClient(claude.Options{
			DefaultModel: claudeModel,
		}),
		gmailClient:   gmail.NewClient(gmail.Options{}),
		gwsClient:     gws.NewClient(gws.Options{}),
		secretManager: secretManager,
		eventsEmit:    runtime.EventsEmit,
	}
	app.authStatus = app.initialAuthStatus()
	app.gmailStatus = app.initialGmailStatus()
	app.claudeStatus = app.initialClaudeStatus()

	return app
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	if err := a.initializeDatabase(); err != nil {
		log.Printf("SQLite 初期化に失敗しました: %v", err)
		return
	}
	if err := a.startScheduler(); err != nil {
		log.Printf("定期実行スケジューラーの起動に失敗しました: %v", err)
	}
}

func (a *App) shutdown(context.Context) {
	a.stopScheduler()

	if err := a.closeDatabase(); err != nil {
		log.Printf("SQLite クローズに失敗しました: %v", err)
	}
}

func (a *App) AppName() string {
	return "Mairu"
}

// GetRuntimeStatus は起動時に必要な初期状態を返す。
func (a *App) GetRuntimeStatus() types.RuntimeStatus {
	a.mu.RLock()
	authStatus := a.authStatus
	gmailStatus := a.gmailStatus
	claudeStatus := a.claudeStatus
	gmailConnected := a.gmailConnected
	gmailAccount := a.gmailAccount
	databaseReady := a.databaseReady
	a.mu.RUnlock()
	lastRunAt := a.loadSchedulerLastRunAt()

	baseContext := a.baseContext()
	googleConfigured := a.authClient.IsConfigured()
	googleTokenPreview := ""
	claudeKeyPreview := ""
	gwsStatus := buildUnavailableGWSStatusMessage()
	gwsAvailable := false

	authorized, err := a.secretManager.HasGoogleToken(baseContext)
	if err != nil {
		authorized = false
		authStatus = buildCredentialErrorMessage("Google 認証状態を確認できません。", err)
	} else if authorized {
		tokenSet, loadErr := a.secretManager.LoadGoogleToken(baseContext)
		if loadErr != nil {
			authorized = false
			authStatus = buildCredentialErrorMessage("Google 認証状態を確認できません。", loadErr)
		} else {
			googleTokenPreview = maskedGoogleTokenPreview(tokenSet)
			if shouldUseStoredAuthMessage(authStatus) {
				authStatus = buildStoredAuthStatusMessage()
			}
		}
	} else if !authorized && shouldUseUnstoredAuthMessage(authStatus) {
		authStatus = buildAuthStatusMessage(googleConfigured)
	}

	if !authorized {
		gmailConnected = false
		gmailAccount = ""
		gmailStatus = buildBlockedGmailStatusMessage()
	} else if !gmailConnected && shouldUseReadyGmailMessage(gmailStatus) {
		gmailStatus = buildReadyGmailStatusMessage()
	}

	claudeConfigured, err := a.secretManager.HasClaudeAPIKey(baseContext)
	if err != nil {
		claudeConfigured = false
		claudeStatus = buildCredentialErrorMessage("Claude API キー状態を確認できません。", err)
	} else if claudeConfigured {
		apiKey, loadErr := a.secretManager.LoadClaudeAPIKey(baseContext)
		if loadErr != nil {
			claudeConfigured = false
			claudeStatus = buildCredentialErrorMessage("Claude API キー状態を確認できません。", loadErr)
		} else {
			claudeKeyPreview = auth.MaskSecret(apiKey)
			if shouldUseStoredClaudeMessage(claudeStatus) {
				claudeStatus = buildStoredClaudeStatusMessage()
			}
		}
	} else if !claudeConfigured && shouldUseUnstoredClaudeMessage(claudeStatus) {
		claudeStatus = buildUnstoredClaudeStatusMessage()
	}

	if a.gwsClient != nil {
		detection := a.gwsClient.Detect()
		gwsAvailable = detection.Available
		if detection.Available {
			gwsStatus = buildAvailableGWSStatusMessage(detection.BinaryPath)
		} else if strings.TrimSpace(detection.Message) != "" {
			gwsStatus = detection.Message
		}
	}

	return types.RuntimeStatus{
		Authorized:         authorized,
		GoogleConfigured:   googleConfigured,
		AuthStatus:         authStatus,
		GoogleTokenPreview: googleTokenPreview,
		GmailConnected:     gmailConnected,
		GmailStatus:        gmailStatus,
		GmailAccountEmail:  gmailAccount,
		ClaudeConfigured:   claudeConfigured,
		ClaudeStatus:       claudeStatus,
		ClaudeKeyPreview:   claudeKeyPreview,
		GWSAvailable:       gwsAvailable,
		GWSStatus:          gwsStatus,
		DatabaseReady:      databaseReady,
		LastRunAt:          lastRunAt,
	}
}

// GetSchedulerSettings は定期実行と通知の設定を返す。
func (a *App) GetSchedulerSettings() types.SchedulerSettings {
	settings := defaultSchedulerSettings()

	store, err := a.requireDBStore()
	if err != nil {
		return settings
	}

	ctx, cancel := context.WithTimeout(a.baseContext(), dbOperationTimeout)
	defer cancel()

	settings.ClassificationIntervalMinutes = int(a.loadSchedulerIntervalMinutes(
		ctx,
		store,
		schedulerSettingClassificationMinutes,
		defaultClassificationIntervalMinutes,
	) / time.Minute)
	settings.BlocklistIntervalMinutes = int(a.loadSchedulerIntervalMinutes(
		ctx,
		store,
		schedulerSettingBlocklistMinutes,
		defaultBlocklistUpdateMinutes,
	) / time.Minute)
	settings.KnownBlockIntervalMinutes = int(a.loadSchedulerIntervalMinutes(
		ctx,
		store,
		schedulerSettingKnownBlockMinutes,
		defaultKnownBlockProcessMinutes,
	) / time.Minute)
	settings.NotificationsEnabled = a.loadSchedulerNotificationsEnabled(ctx, store)

	return settings
}

// UpdateSchedulerSettings は定期実行と通知の設定を保存し、スケジューラーへ即時反映する。
func (a *App) UpdateSchedulerSettings(request types.UpdateSchedulerSettingsRequest) types.OperationResult {
	store, err := a.requireDBStore()
	if err != nil {
		return types.OperationResult{
			Success: false,
			Message: err.Error(),
		}
	}

	previous := a.GetSchedulerSettings()
	next := types.SchedulerSettings{
		ClassificationIntervalMinutes: normalizeSchedulerIntervalMinutes(
			request.ClassificationIntervalMinutes,
			defaultClassificationIntervalMinutes,
		),
		BlocklistIntervalMinutes: normalizeSchedulerIntervalMinutes(
			request.BlocklistIntervalMinutes,
			defaultBlocklistUpdateMinutes,
		),
		KnownBlockIntervalMinutes: normalizeSchedulerIntervalMinutes(
			request.KnownBlockIntervalMinutes,
			defaultKnownBlockProcessMinutes,
		),
		NotificationsEnabled: request.NotificationsEnabled,
	}

	ctx, cancel := context.WithTimeout(a.baseContext(), dbOperationTimeout)
	defer cancel()

	if err := store.SetSettings(ctx, schedulerSettingsValueMap(next)); err != nil {
		return types.OperationResult{
			Success: false,
			Message: fmt.Sprintf("自動実行設定を保存できませんでした: %v", err),
		}
	}

	a.stopScheduler()
	if err := a.startScheduler(); err != nil {
		rollbackContext, rollbackCancel := context.WithTimeout(a.baseContext(), dbOperationTimeout)
		defer rollbackCancel()

		if rollbackErr := store.SetSettings(rollbackContext, schedulerSettingsValueMap(previous)); rollbackErr != nil {
			return types.OperationResult{
				Success: false,
				Message: fmt.Sprintf(
					"設定反映に失敗し、ロールバックにも失敗しました: apply=%v rollback=%v",
					err,
					rollbackErr,
				),
			}
		}

		if restartErr := a.startScheduler(); restartErr != nil {
			return types.OperationResult{
				Success: false,
				Message: fmt.Sprintf(
					"設定反映に失敗し旧設定へ戻しましたが、scheduler 再起動に失敗しました: apply=%v restart=%v",
					err,
					restartErr,
				),
			}
		}

		return types.OperationResult{
			Success: false,
			Message: fmt.Sprintf("設定反映に失敗したため、旧設定へロールバックしました: %v", err),
		}
	}

	return types.OperationResult{
		Success: true,
		Message: "自動実行設定を保存しました。",
	}
}

// StartGoogleLogin は Google OAuth PKCE ログインを開始し、トークン保存まで完了する。
func (a *App) StartGoogleLogin() (types.GoogleLoginResult, error) {
	if !a.authClient.IsConfigured() {
		a.setAuthStatus(buildAuthStatusMessage(false))
		return types.GoogleLoginResult{
			Success: false,
			Message: buildAuthStatusMessage(false),
			Scopes:  a.authClient.Scopes(),
		}, nil
	}

	if a.hasLoginInProgress() {
		message := "Google ログインはすでに進行中です。完了または中断してから再試行してください。"
		a.setAuthStatus(message)
		return types.GoogleLoginResult{
			Success: false,
			Message: message,
			Scopes:  a.authClient.Scopes(),
		}, nil
	}

	loginContext, cancel := context.WithCancel(a.baseContext())
	defer cancel()
	cancelSeq, ok := a.setLoginCancelIfIdle(cancel)
	if !ok {
		cancel()
		message := "Google ログインはすでに進行中です。完了または中断してから再試行してください。"
		a.setAuthStatus(message)
		return types.GoogleLoginResult{
			Success: false,
			Message: message,
			Scopes:  a.authClient.Scopes(),
		}, nil
	}
	defer a.clearLoginCancel(cancelSeq)
	a.setAuthStatus("ブラウザを開いて Google ログインを待っています。")

	loginResult, err := a.authClient.RunLoginFlow(loginContext)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			message := "ログイン処理を中断しました。再試行できます。"
			a.setAuthStatus(message)
			return types.GoogleLoginResult{
				Success: false,
				Message: message,
				Scopes:  a.authClient.Scopes(),
			}, nil
		}

		a.setAuthStatus(buildCredentialErrorMessage("Google ログインに失敗しました。", err))
		return types.GoogleLoginResult{}, err
	}

	tokenSet, err := a.authClient.ExchangeCode(loginContext, loginResult)
	if err != nil {
		a.setAuthStatus(buildCredentialErrorMessage("Google トークン交換に失敗しました。", err))
		return types.GoogleLoginResult{}, err
	}

	if err := a.secretManager.SaveGoogleToken(loginContext, tokenSet); err != nil {
		message := buildCredentialErrorMessage("Google トークンをキーチェーンへ保存できませんでした。", err)
		a.setAuthStatus(message)
		return types.GoogleLoginResult{}, err
	}

	message := buildGoogleTokenSavedStatusMessage()
	a.setAuthStatus(message)
	a.setGmailConnectionState(false, buildReadyGmailStatusMessage(), "")

	scopes := tokenSet.Scopes()
	if len(scopes) == 0 {
		scopes = loginResult.Scopes
	}

	return types.GoogleLoginResult{
		Success:            true,
		Message:            message,
		AuthorizationURL:   loginResult.AuthorizationURL,
		RedirectURL:        loginResult.RedirectURL,
		TokenStored:        true,
		RefreshTokenStored: strings.TrimSpace(tokenSet.RefreshToken) != "",
		StoredPreview:      auth.MaskSecret(tokenSet.RefreshToken),
		Scopes:             scopes,
	}, nil
}

// CheckGmailConnection は保存済みトークンで Gmail API への接続確認を行う。
func (a *App) CheckGmailConnection() types.GmailConnectionResult {
	baseContext, cancel := context.WithTimeout(a.baseContext(), gmailConnectionTimeout)
	defer cancel()

	token, err := a.secretManager.LoadGoogleToken(baseContext)
	if err != nil {
		message := buildCredentialErrorMessage("保存済み Google トークンを読み出せませんでした。", err)
		a.setGmailConnectionState(false, message, "")
		return types.GmailConnectionResult{
			Success: false,
			Message: message,
		}
	}

	token, refreshed, err := a.authClient.EnsureValidToken(baseContext, token)
	if err != nil {
		message := buildCredentialErrorMessage("Google トークンを再利用できませんでした。", err)
		a.setGmailConnectionState(false, message, "")
		return types.GmailConnectionResult{
			Success: false,
			Message: message,
		}
	}

	if refreshed {
		if err := a.secretManager.SaveGoogleToken(baseContext, token); err != nil {
			message := buildCredentialErrorMessage("更新した Google トークンをキーチェーンへ保存できませんでした。", err)
			a.setGmailConnectionState(false, message, "")
			return types.GmailConnectionResult{
				Success: false,
				Message: message,
			}
		}
	}

	profile, err := a.gmailClient.CheckConnection(baseContext, token.AccessToken)
	if err != nil {
		message := buildCredentialErrorMessage("Gmail API へ接続できませんでした。", err)
		a.setGmailConnectionState(false, message, "")
		return types.GmailConnectionResult{
			Success: false,
			Message: message,
		}
	}

	message := buildGmailConnectedStatusMessage(profile.EmailAddress)
	a.setGmailConnectionState(true, message, profile.EmailAddress)

	return types.GmailConnectionResult{
		Success:        true,
		Message:        message,
		EmailAddress:   profile.EmailAddress,
		MessagesTotal:  profile.MessagesTotal,
		ThreadsTotal:   profile.ThreadsTotal,
		HistoryID:      profile.HistoryID,
		TokenRefreshed: refreshed,
	}
}

// CheckGWSDiagnostics は gws の導入状態とバージョン取得を診断する。
func (a *App) CheckGWSDiagnostics() types.GWSDiagnosticsResult {
	if a.gwsClient == nil {
		return types.GWSDiagnosticsResult{
			Success:   false,
			Available: false,
			Message:   "gws クライアントが初期化されていません。",
			ErrorKind: types.GWSCLIErrorKindExecution,
		}
	}

	ctx, cancel := context.WithTimeout(a.baseContext(), gwsCommandTimeout)
	defer cancel()

	result := a.gwsClient.Diagnose(ctx)
	return types.GWSDiagnosticsResult{
		Success:    result.Success,
		Available:  result.Available,
		Message:    result.Message,
		BinaryPath: result.BinaryPath,
		Version:    result.Version,
		Command:    result.Command,
		Output:     result.Output,
		ErrorKind:  mapGWSErrorKind(result.ErrorKind),
	}
}

// PreviewGWSGmailDryRun は gws Gmail read-only dry-run の PoC を実行する。
func (a *App) PreviewGWSGmailDryRun(request types.GWSGmailDryRunRequest) types.GWSGmailDryRunResult {
	if a.gwsClient == nil {
		return types.GWSGmailDryRunResult{
			Success:   false,
			Message:   "gws クライアントが初期化されていません。",
			ErrorKind: types.GWSCLIErrorKindExecution,
		}
	}

	ctx, cancel := context.WithTimeout(a.baseContext(), gwsCommandTimeout)
	defer cancel()

	result := a.gwsClient.RunGmailListDryRun(ctx, gws.GmailDryRunRequest{
		Query:      request.Query,
		MaxResults: request.MaxResults,
	})

	return types.GWSGmailDryRunResult{
		Success:    result.Success,
		Message:    result.Message,
		BinaryPath: result.BinaryPath,
		Command:    result.Command,
		Output:     result.Output,
		ErrorKind:  mapGWSErrorKind(result.ErrorKind),
	}
}

// ExecuteGmailActions は承認済み分類結果を Gmail 側へ反映する。
func (a *App) ExecuteGmailActions(
	request types.ExecuteGmailActionsRequest,
) (types.ExecuteGmailActionsResult, error) {
	if !request.Confirmed {
		return types.ExecuteGmailActionsResult{
			Success: false,
			Message: "Gmail アクション実行前に確認ステップを完了してください。",
		}, nil
	}
	if len(request.Decisions) == 0 {
		return types.ExecuteGmailActionsResult{
			Success: false,
			Message: "実行対象のメールが選択されていません。",
		}, nil
	}

	baseContext, cancel := context.WithTimeout(a.baseContext(), gmailActionTimeout)
	defer cancel()

	token, err := a.secretManager.LoadGoogleToken(baseContext)
	if err != nil {
		return types.ExecuteGmailActionsResult{}, fmt.Errorf("保存済み Google トークンを読み出せませんでした: %w", err)
	}

	token, refreshed, err := a.authClient.EnsureValidToken(baseContext, token)
	if err != nil {
		return types.ExecuteGmailActionsResult{}, fmt.Errorf("Google トークンを再利用できませんでした: %w", err)
	}
	if refreshed {
		if err := a.secretManager.SaveGoogleToken(baseContext, token); err != nil {
			return types.ExecuteGmailActionsResult{}, fmt.Errorf("更新した Google トークンをキーチェーンへ保存できませんでした: %w", err)
		}
	}

	store, storeErr := a.requireDBStore()
	if storeErr != nil {
		return types.ExecuteGmailActionsResult{}, fmt.Errorf("重複防止に必要な DB ストアを初期化できませんでした: %w", storeErr)
	}

	executionRequest := request
	skippedLogEntries := make([]types.ActionLogEntry, 0)
	executionRequest, skippedLogEntries, err = a.excludeAlreadySucceededActions(
		baseContext,
		store,
		request,
	)
	if err != nil {
		return types.ExecuteGmailActionsResult{}, fmt.Errorf("重複防止の事前判定に失敗しました: %w", err)
	}

	result := types.ExecuteGmailActionsResult{
		Success: true,
		Message: "実行対象がありませんでした。",
	}
	if len(executionRequest.Decisions) > 0 {
		result, err = a.gmailClient.ExecuteActions(baseContext, token.AccessToken, executionRequest.Decisions)
		if err != nil {
			return types.ExecuteGmailActionsResult{}, err
		}
	}

	result = mergeGmailActionSkippedResult(result, len(request.Decisions), len(skippedLogEntries))

	logEntries, buildErr := buildActionLogEntries(executionRequest, result)
	if buildErr != nil {
		log.Printf("処理ログ生成に失敗しました: %v", buildErr)
		result.Message = result.Message + " 処理ログ保存はスキップされました。"
	} else {
		logEntries = append(logEntries, skippedLogEntries...)
	}
	if len(logEntries) > 0 {
		logContext, logCancel := context.WithTimeout(a.baseContext(), dbOperationTimeout)
		defer logCancel()

		if err := store.RecordActionLogEntries(logContext, logEntries); err != nil {
			log.Printf("処理ログ保存に失敗しました: %v", err)
			result.Message = result.Message + " 処理ログ保存に失敗しました。"
		}
	}

	result.TokenRefreshed = refreshed
	account := a.currentGmailAccount()
	a.setGmailConnectionState(true, result.Message, account)

	return result, nil
}

// ClassifyEmails は保存済み Claude API キーでメール分類を実行する。
func (a *App) ClassifyEmails(request types.ClassificationRequest) (types.ClassificationResponse, error) {
	baseContext, cancel := context.WithTimeout(a.baseContext(), claudeClassificationTimeout)
	defer cancel()

	unblockedMessages, skippedResults, err := a.classifyByBlocklist(baseContext, request.Messages)
	if err != nil {
		return types.ClassificationResponse{}, err
	}
	if len(unblockedMessages) == 0 && len(skippedResults) > 0 {
		return types.ClassificationResponse{
			Model:   "blocklist-skip",
			Results: skippedResults,
		}, nil
	}

	apiKey, err := a.secretManager.LoadClaudeAPIKey(baseContext)
	if err != nil {
		return types.ClassificationResponse{}, fmt.Errorf("保存済み Claude API キーを読み出せませんでした: %w", err)
	}

	client := a.claudeClient
	if client == nil {
		client = claude.NewClient(claude.Options{})
	}

	response, err := client.Classify(baseContext, apiKey, types.ClassificationRequest{
		Model:    request.Model,
		Messages: unblockedMessages,
	})
	if err != nil {
		return types.ClassificationResponse{}, err
	}

	for index := range response.Results {
		if !response.Results[index].Source.IsValid() {
			response.Results[index].Source = types.ClassificationSourceClaude
		}
	}

	response.Results = mergeClassificationResults(
		request.Messages,
		response.Results,
		skippedResults,
	)

	return response, nil
}

// GetBlocklistEntries は登録済みブロックリストを返す。
func (a *App) GetBlocklistEntries() ([]types.BlocklistEntry, error) {
	store, err := a.requireDBStore()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(a.baseContext(), dbOperationTimeout)
	defer cancel()

	return store.ListBlocklistEntries(ctx)
}

// UpsertBlocklistEntry は sender/domain ブロックを追加または更新する。
func (a *App) UpsertBlocklistEntry(request types.UpsertBlocklistEntryRequest) types.BlocklistOperationResult {
	store, err := a.requireDBStore()
	if err != nil {
		return types.BlocklistOperationResult{
			Success: false,
			Message: err.Error(),
		}
	}

	ctx, cancel := context.WithTimeout(a.baseContext(), dbOperationTimeout)
	defer cancel()

	entry, err := store.UpsertBlocklistEntry(ctx, request.Kind, request.Pattern, request.Note)
	if err != nil {
		return types.BlocklistOperationResult{
			Success: false,
			Message: fmt.Sprintf("ブロックリスト保存に失敗しました: %v", err),
		}
	}

	return types.BlocklistOperationResult{
		Success: true,
		Message: fmt.Sprintf("ブロックリストを保存しました (%s: %s)", entry.Kind, entry.Pattern),
	}
}

// DeleteBlocklistEntry は ID 指定でブロックリストを削除する。
func (a *App) DeleteBlocklistEntry(id int64) types.BlocklistOperationResult {
	store, err := a.requireDBStore()
	if err != nil {
		return types.BlocklistOperationResult{
			Success: false,
			Message: err.Error(),
		}
	}

	ctx, cancel := context.WithTimeout(a.baseContext(), dbOperationTimeout)
	defer cancel()

	deleted, err := store.DeleteBlocklistEntry(ctx, id)
	if err != nil {
		return types.BlocklistOperationResult{
			Success: false,
			Message: fmt.Sprintf("ブロックリスト削除に失敗しました: %v", err),
		}
	}
	if !deleted {
		return types.BlocklistOperationResult{
			Success: false,
			Message: "対象のブロックリストが見つかりませんでした。",
		}
	}

	return types.BlocklistOperationResult{
		Success: true,
		Message: "ブロックリストを削除しました。",
	}
}

// RecordClassificationCorrection は分類修正履歴を保存する。
func (a *App) RecordClassificationCorrection(
	correction types.ClassificationCorrection,
) types.BlocklistOperationResult {
	store, err := a.requireDBStore()
	if err != nil {
		return types.BlocklistOperationResult{
			Success: false,
			Message: err.Error(),
		}
	}

	ctx, cancel := context.WithTimeout(a.baseContext(), dbOperationTimeout)
	defer cancel()

	if err := store.RecordClassificationCorrection(ctx, correction); err != nil {
		return types.BlocklistOperationResult{
			Success: false,
			Message: fmt.Sprintf("分類修正履歴を保存できませんでした: %v", err),
		}
	}

	return types.BlocklistOperationResult{
		Success: true,
		Message: "分類修正履歴を保存しました。",
	}
}

// RecordClassificationRun は分類結果をエクスポート用ログへ保存する。
func (a *App) RecordClassificationRun(request types.RecordClassificationRunRequest) types.OperationResult {
	store, err := a.requireDBStore()
	if err != nil {
		return types.OperationResult{
			Success: false,
			Message: err.Error(),
		}
	}

	entries, err := buildClassificationLogEntries(request)
	if err != nil {
		return types.OperationResult{
			Success: false,
			Message: fmt.Sprintf("分類ログの整形に失敗しました: %v", err),
		}
	}

	ctx, cancel := context.WithTimeout(a.baseContext(), dbOperationTimeout)
	defer cancel()

	if err := store.RecordClassificationLogEntries(ctx, entries); err != nil {
		return types.OperationResult{
			Success: false,
			Message: fmt.Sprintf("分類ログを保存できませんでした: %v", err),
		}
	}

	return types.OperationResult{
		Success: true,
		Message: fmt.Sprintf("分類ログを %d 件保存しました。", len(entries)),
	}
}

// ExportProcessedMailCSV は処理済みメールログを CSV へ出力する。
func (a *App) ExportProcessedMailCSV() types.OperationResult {
	return a.exportActionLogs(
		"処理済みメール一覧 (CSV) を保存",
		"mairu-processed-mails",
		".csv",
		[]runtime.FileFilter{{DisplayName: "CSV ファイル", Pattern: "*.csv"}},
		func(entries []types.ActionLogEntry, exportedAt time.Time) ([]byte, error) {
			return exporter.MarshalProcessedMailCSV(entries)
		},
	)
}

// ExportProcessedMailJSON は処理済みメールログを JSON へ出力する。
func (a *App) ExportProcessedMailJSON() types.OperationResult {
	return a.exportActionLogs(
		"処理済みメール一覧 (JSON) を保存",
		"mairu-processed-mails",
		".json",
		[]runtime.FileFilter{{DisplayName: "JSON ファイル", Pattern: "*.json"}},
		exporter.MarshalProcessedMailJSON,
	)
}

// ExportBlocklistJSON は blocklist を JSON へ出力する。
func (a *App) ExportBlocklistJSON() types.OperationResult {
	store, err := a.requireDBStore()
	if err != nil {
		return types.OperationResult{Success: false, Message: err.Error()}
	}

	ctx, cancel := context.WithTimeout(a.baseContext(), dbOperationTimeout)
	defer cancel()

	entries, err := store.ListBlocklistEntries(ctx)
	if err != nil {
		return types.OperationResult{
			Success: false,
			Message: fmt.Sprintf("blocklist を取得できませんでした: %v", err),
		}
	}

	exportedAt := time.Now()
	data, err := exporter.MarshalBlocklistJSON(entries, exportedAt)
	if err != nil {
		return types.OperationResult{
			Success: false,
			Message: fmt.Sprintf("blocklist JSON を生成できませんでした: %v", err),
		}
	}

	return a.saveExportFile(
		"ブロックリスト JSON を保存",
		buildExportFilename("mairu-blocklist", exportedAt, ".json"),
		[]runtime.FileFilter{{DisplayName: "JSON ファイル", Pattern: "*.json"}},
		data,
	)
}

// ImportBlocklistJSON は JSON ファイルから blocklist を取り込む。
func (a *App) ImportBlocklistJSON() types.OperationResult {
	store, err := a.requireDBStore()
	if err != nil {
		return types.OperationResult{Success: false, Message: err.Error()}
	}

	path, err := runtime.OpenFileDialog(a.baseContext(), runtime.OpenDialogOptions{
		Title:            "取り込むブロックリスト JSON を選択",
		DefaultDirectory: defaultExportDirectory(),
		Filters:          []runtime.FileFilter{{DisplayName: "JSON ファイル", Pattern: "*.json"}},
	})
	if err != nil {
		return types.OperationResult{
			Success: false,
			Message: fmt.Sprintf("ファイル選択ダイアログを開けませんでした: %v", err),
		}
	}
	if strings.TrimSpace(path) == "" {
		return types.OperationResult{
			Success: false,
			Message: "blocklist JSON の取り込みをキャンセルしました。",
		}
	}

	payload, err := os.ReadFile(path)
	if err != nil {
		return types.OperationResult{
			Success: false,
			Message: fmt.Sprintf("選択した JSON を読み込めませんでした: %v", err),
		}
	}

	entries, err := exporter.ParseBlocklistJSON(payload)
	if err != nil {
		return types.OperationResult{
			Success: false,
			Message: fmt.Sprintf("blocklist JSON を解釈できませんでした: %v", err),
		}
	}

	ctx, cancel := context.WithTimeout(a.baseContext(), dbOperationTimeout)
	defer cancel()

	imported, err := store.ImportBlocklistEntries(ctx, entries)
	if err != nil {
		return types.OperationResult{
			Success: false,
			Message: fmt.Sprintf("blocklist を取り込めませんでした: %v", err),
		}
	}

	return types.OperationResult{
		Success: true,
		Message: fmt.Sprintf("blocklist を %d 件取り込みました。", imported),
	}
}

// ExportImportantSummaryCSV は重要メールサマリーを CSV へ出力する。
func (a *App) ExportImportantSummaryCSV() types.OperationResult {
	return a.exportClassificationLogs(
		"重要メールサマリー (CSV) を保存",
		"mairu-important-summary",
		".csv",
		[]runtime.FileFilter{{DisplayName: "CSV ファイル", Pattern: "*.csv"}},
		func(entries []types.ClassificationLogEntry, exportedAt time.Time) ([]byte, error) {
			return exporter.MarshalImportantSummaryCSV(entries)
		},
	)
}

// ExportImportantSummaryPDF は重要メールサマリーを PDF へ出力する。
func (a *App) ExportImportantSummaryPDF() types.OperationResult {
	return a.exportClassificationLogs(
		"重要メールサマリー (PDF) を保存",
		"mairu-important-summary",
		".pdf",
		[]runtime.FileFilter{{DisplayName: "PDF ファイル", Pattern: "*.pdf"}},
		exporter.MarshalImportantSummaryPDF,
	)
}

// ExportDailyLogsCSV は日別分類ログを CSV へ出力する。
func (a *App) ExportDailyLogsCSV() types.OperationResult {
	return a.exportClassificationLogs(
		"日別分類ログ (CSV) を保存",
		"mairu-daily-logs",
		".csv",
		[]runtime.FileFilter{{DisplayName: "CSV ファイル", Pattern: "*.csv"}},
		func(entries []types.ClassificationLogEntry, exportedAt time.Time) ([]byte, error) {
			return exporter.MarshalDailyLogsCSV(entries)
		},
	)
}

// ExportDailyLogsJSON は日別分類ログを JSON へ出力する。
func (a *App) ExportDailyLogsJSON() types.OperationResult {
	return a.exportClassificationLogs(
		"日別分類ログ (JSON) を保存",
		"mairu-daily-logs",
		".json",
		[]runtime.FileFilter{{DisplayName: "JSON ファイル", Pattern: "*.json"}},
		exporter.MarshalDailyLogsJSON,
	)
}

// GetBlocklistSuggestions は修正履歴からブロック候補を返す。
func (a *App) GetBlocklistSuggestions() ([]types.BlocklistSuggestion, error) {
	store, err := a.requireDBStore()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(a.baseContext(), dbOperationTimeout)
	defer cancel()

	return store.ListBlocklistSuggestions(ctx, blocklistSuggestionMinimum)
}

// CancelGoogleLogin は進行中の Google ログインを中断する。
func (a *App) CancelGoogleLogin() bool {
	a.mu.Lock()
	cancel := a.loginCancel
	a.loginCancel = nil
	a.loginCancelSeq++
	a.mu.Unlock()

	if cancel == nil {
		return false
	}

	cancel()
	a.setAuthStatus("ログイン処理を中断しました。再試行できます。")
	return true
}

// SaveClaudeAPIKey は Claude API キーを OS キーチェーンへ保存する。
func (a *App) SaveClaudeAPIKey(apiKey string) types.SecretOperationResult {
	if err := a.secretManager.SaveClaudeAPIKey(a.baseContext(), apiKey); err != nil {
		message := buildCredentialErrorMessage("Claude API キーをキーチェーンへ保存できませんでした。", err)
		a.setClaudeStatus(message)
		return types.SecretOperationResult{
			Success: false,
			Message: message,
		}
	}

	message := buildClaudeAPIKeySavedStatusMessage()
	a.setClaudeStatus(message)
	return types.SecretOperationResult{
		Success: true,
		Message: message,
	}
}

// ClearClaudeAPIKey は保存済み Claude API キーを削除する。
func (a *App) ClearClaudeAPIKey() types.SecretOperationResult {
	if err := a.secretManager.DeleteClaudeAPIKey(a.baseContext()); err != nil {
		message := buildCredentialErrorMessage("Claude API キーをキーチェーンから削除できませんでした。", err)
		a.setClaudeStatus(message)
		return types.SecretOperationResult{
			Success: false,
			Message: message,
		}
	}

	message := "Claude API キーをキーチェーンから削除しました。"
	a.setClaudeStatus(message)
	return types.SecretOperationResult{
		Success: true,
		Message: message,
	}
}

func (a *App) setAuthStatus(status string) {
	a.mu.Lock()
	a.authStatus = status
	a.mu.Unlock()
}

func (a *App) setClaudeStatus(status string) {
	a.mu.Lock()
	a.claudeStatus = status
	a.mu.Unlock()
}

func (a *App) setGmailConnectionState(connected bool, status string, account string) {
	a.mu.Lock()
	a.gmailConnected = connected
	a.gmailStatus = status
	a.gmailAccount = account
	a.mu.Unlock()
}

func (a *App) setDatabaseState(store *db.Store, ready bool) {
	a.mu.Lock()
	a.dbStore = store
	a.databaseReady = ready
	a.mu.Unlock()
}

func (a *App) hasLoginInProgress() bool {
	a.mu.RLock()
	running := a.loginCancel != nil
	a.mu.RUnlock()
	return running
}

func (a *App) currentGmailAccount() string {
	a.mu.RLock()
	account := a.gmailAccount
	a.mu.RUnlock()
	return account
}

func (a *App) setLoginCancelIfIdle(cancel context.CancelFunc) (uint64, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.loginCancel != nil {
		return 0, false
	}

	a.loginCancelSeq++
	a.loginCancel = cancel
	return a.loginCancelSeq, true
}

func (a *App) clearLoginCancel(seq uint64) {
	a.mu.Lock()
	if a.loginCancelSeq == seq {
		a.loginCancel = nil
	}
	a.mu.Unlock()
}

func buildAuthStatusMessage(configured bool) string {
	if !configured {
		return "環境変数 MAIRU_GOOGLE_OAUTH_CLIENT_ID と MAIRU_GOOGLE_OAUTH_CLIENT_SECRET を設定すると Google ログインを開始できます。"
	}

	return "Google ログインを開始すると、localhost で認可コードを受け取ってキーチェーンに保存します。"
}

func buildStoredAuthStatusMessage() string {
	return "Google トークンをキーチェーンに保存済みです。"
}

func buildGoogleTokenSavedStatusMessage() string {
	return "Google トークンをキーチェーンに保存しました。続けて Gmail 接続確認を実行できます。"
}

func buildReadyGmailStatusMessage() string {
	return "保存済み Google トークンで Gmail 接続確認を実行できます。"
}

func buildBlockedGmailStatusMessage() string {
	return "Google ログイン後に Gmail 接続確認を実行できます。"
}

func buildGmailConnectedStatusMessage(emailAddress string) string {
	trimmed := strings.TrimSpace(emailAddress)
	if trimmed == "" {
		return "Gmail API への接続確認に成功しました。"
	}

	return fmt.Sprintf("Gmail API への接続確認に成功しました。接続先: %s", trimmed)
}

func buildStoredClaudeStatusMessage() string {
	return "Claude API キーをキーチェーンに保存済みです。"
}

func buildClaudeAPIKeySavedStatusMessage() string {
	return "Claude API キーをキーチェーンに保存しました。"
}

func buildUnstoredClaudeStatusMessage() string {
	return "Claude API キーを保存すると、次の分類機能から利用できます。"
}

func buildAvailableGWSStatusMessage(binaryPath string) string {
	trimmed := strings.TrimSpace(binaryPath)
	if trimmed == "" {
		return "gws を利用できます。"
	}
	return fmt.Sprintf("gws を利用できます (%s)", trimmed)
}

func buildUnavailableGWSStatusMessage() string {
	return "gws は未導入です。必要な場合のみインストールしてください。"
}

func buildCredentialErrorMessage(prefix string, err error) string {
	log.Printf("%s detail=%v", prefix, err)
	return prefix + " 詳細はアプリのログを確認してください。"
}

func mapGWSErrorKind(kind gws.ErrorKind) types.GWSCLIErrorKind {
	switch kind {
	case gws.ErrorKindNone:
		return types.GWSCLIErrorKindNone
	case gws.ErrorKindNotInstalled:
		return types.GWSCLIErrorKindNotInstalled
	case gws.ErrorKindAuth:
		return types.GWSCLIErrorKindAuth
	case gws.ErrorKindInvalidCommand:
		return types.GWSCLIErrorKindInvalidCommand
	case gws.ErrorKindTimeout:
		return types.GWSCLIErrorKindTimeout
	default:
		return types.GWSCLIErrorKindExecution
	}
}

func shouldUseStoredAuthMessage(message string) bool {
	trimmed := strings.TrimSpace(message)
	return trimmed == "" ||
		trimmed == buildAuthStatusMessage(true) ||
		trimmed == buildGoogleTokenSavedStatusMessage() ||
		trimmed == buildStoredAuthStatusMessage()
}

func shouldUseUnstoredAuthMessage(message string) bool {
	trimmed := strings.TrimSpace(message)
	return trimmed == "" ||
		trimmed == buildAuthStatusMessage(true) ||
		trimmed == buildGoogleTokenSavedStatusMessage() ||
		trimmed == buildStoredAuthStatusMessage()
}

func shouldUseStoredClaudeMessage(message string) bool {
	trimmed := strings.TrimSpace(message)
	return trimmed == "" ||
		trimmed == buildClaudeAPIKeySavedStatusMessage() ||
		trimmed == buildStoredClaudeStatusMessage()
}

func shouldUseUnstoredClaudeMessage(message string) bool {
	trimmed := strings.TrimSpace(message)
	return trimmed == "" ||
		trimmed == buildClaudeAPIKeySavedStatusMessage() ||
		trimmed == buildStoredClaudeStatusMessage()
}

func shouldUseReadyGmailMessage(message string) bool {
	trimmed := strings.TrimSpace(message)
	return trimmed == "" ||
		trimmed == buildReadyGmailStatusMessage() ||
		trimmed == buildBlockedGmailStatusMessage()
}

func maskedGoogleTokenPreview(token auth.TokenSet) string {
	if preview := auth.MaskSecret(token.RefreshToken); preview != "" {
		return preview
	}
	return auth.MaskSecret(token.AccessToken)
}

func (a *App) baseContext() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

func (a *App) initializeDatabase() error {
	a.stopScheduler()

	if err := a.closeDatabase(); err != nil {
		return err
	}

	store, err := db.Open(a.baseContext(), db.OpenOptions{
		AppName: a.AppName(),
	})
	if err != nil {
		a.setDatabaseState(nil, false)
		return err
	}

	a.setDatabaseState(store, true)
	return nil
}

func (a *App) closeDatabase() error {
	a.mu.Lock()
	store := a.dbStore
	a.dbStore = nil
	a.databaseReady = false
	a.mu.Unlock()

	if store == nil {
		return nil
	}

	return store.Close()
}

func (a *App) requireDBStore() (*db.Store, error) {
	a.mu.RLock()
	store := a.dbStore
	ready := a.databaseReady
	a.mu.RUnlock()

	if !ready || store == nil {
		return nil, errors.New("SQLite が初期化されていないためローカルデータ操作を実行できません")
	}

	return store, nil
}

func (a *App) startScheduler() error {
	store, err := a.requireDBStore()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(a.baseContext(), dbOperationTimeout)
	defer cancel()

	classificationInterval := a.loadSchedulerIntervalMinutes(
		ctx,
		store,
		schedulerSettingClassificationMinutes,
		defaultClassificationIntervalMinutes,
	)
	blocklistInterval := a.loadSchedulerIntervalMinutes(
		ctx,
		store,
		schedulerSettingBlocklistMinutes,
		defaultBlocklistUpdateMinutes,
	)
	knownBlockInterval := a.loadSchedulerIntervalMinutes(
		ctx,
		store,
		schedulerSettingKnownBlockMinutes,
		defaultKnownBlockProcessMinutes,
	)

	jobs := []scheduler.Job{
		{
			ID:           schedulerJobBlocklist,
			Interval:     blocklistInterval,
			MaxRetries:   maxSchedulerRetries,
			RetryBackoff: schedulerRetryBackoffBase,
			Handler:      a.runScheduledBlocklist,
		},
		{
			ID:           schedulerJobClassification,
			Interval:     classificationInterval,
			MaxRetries:   maxSchedulerRetries,
			RetryBackoff: schedulerRetryBackoffBase,
			Handler:      a.runScheduledClassification,
		},
	}
	if a.runScheduledKnownBlockJob != nil {
		jobs = append(jobs, scheduler.Job{
			ID:           schedulerJobKnownBlock,
			Interval:     knownBlockInterval,
			MaxRetries:   maxSchedulerRetries,
			RetryBackoff: schedulerRetryBackoffBase,
			Handler:      a.runScheduledKnownBlock,
		})
	} else {
		log.Printf("[scheduler] job=%s は未実装のため登録をスキップしました", schedulerJobKnownBlock)
	}

	service, err := scheduler.New(scheduler.Options{
		Jobs:    jobs,
		OnEvent: a.logSchedulerEvent,
	})
	if err != nil {
		return err
	}

	runCtx, runCancel := context.WithCancel(a.baseContext())
	if err := service.Start(runCtx); err != nil {
		runCancel()
		return err
	}

	a.mu.Lock()
	a.schedulerSvc = service
	a.schedulerStop = runCancel
	a.mu.Unlock()

	return nil
}

func (a *App) stopScheduler() {
	a.mu.Lock()
	service := a.schedulerSvc
	cancel := a.schedulerStop
	a.schedulerSvc = nil
	a.schedulerStop = nil
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if service != nil {
		service.Stop()
	}
}

func (a *App) loadSchedulerIntervalMinutes(
	ctx context.Context,
	store *db.Store,
	settingKey string,
	defaultMinutes int,
) time.Duration {
	value, ok, err := store.GetSetting(ctx, settingKey)
	if err != nil {
		log.Printf("scheduler 設定読み出しに失敗しました key=%s: %v", settingKey, err)
		return time.Duration(defaultMinutes) * time.Minute
	}
	if !ok {
		return time.Duration(defaultMinutes) * time.Minute
	}

	minutes, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		log.Printf("scheduler 設定の形式が不正です key=%s value=%q", settingKey, value)
		return time.Duration(defaultMinutes) * time.Minute
	}
	minutes = normalizeSchedulerIntervalMinutes(minutes, defaultMinutes)

	return time.Duration(minutes) * time.Minute
}

func (a *App) loadSchedulerNotificationsEnabled(ctx context.Context, store *db.Store) bool {
	value, ok, err := store.GetSetting(ctx, schedulerSettingNotificationsEnabled)
	if err != nil {
		log.Printf("scheduler 通知設定の読み出しに失敗しました: %v", err)
		return true
	}
	if !ok {
		return true
	}

	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		log.Printf("scheduler 通知設定の形式が不正です value=%q", value)
		return true
	}
}

func defaultSchedulerSettings() types.SchedulerSettings {
	return types.SchedulerSettings{
		ClassificationIntervalMinutes: defaultClassificationIntervalMinutes,
		BlocklistIntervalMinutes:      defaultBlocklistUpdateMinutes,
		KnownBlockIntervalMinutes:     defaultKnownBlockProcessMinutes,
		NotificationsEnabled:          true,
	}
}

func normalizeSchedulerIntervalMinutes(value int, defaultValue int) int {
	minutes := value
	if minutes <= 0 {
		minutes = defaultValue
	}
	if minutes < minSchedulerIntervalMinutes {
		return minSchedulerIntervalMinutes
	}
	return minutes
}

func schedulerSettingsValueMap(settings types.SchedulerSettings) map[string]string {
	return map[string]string{
		schedulerSettingClassificationMinutes: strconv.Itoa(settings.ClassificationIntervalMinutes),
		schedulerSettingBlocklistMinutes:      strconv.Itoa(settings.BlocklistIntervalMinutes),
		schedulerSettingKnownBlockMinutes:     strconv.Itoa(settings.KnownBlockIntervalMinutes),
		schedulerSettingNotificationsEnabled:  strconv.FormatBool(settings.NotificationsEnabled),
	}
}

func (a *App) runScheduledClassification(ctx context.Context) (scheduler.Result, error) {
	if a.runScheduledClassificationJob != nil {
		return a.runScheduledClassificationJob(ctx)
	}

	lastRunAt := a.loadSchedulerJobLastRunAt(schedulerJobClassification)
	if lastRunAt == nil {
		return scheduler.Result{
			Skipped: true,
			Message: schedulerMessageMissingClassificationLastRun,
		}, nil
	}

	hasGoogleToken, err := a.secretManager.HasGoogleToken(ctx)
	if err != nil {
		return scheduler.Result{}, fmt.Errorf("Google トークン状態を確認できませんでした: %w", err)
	}
	hasClaudeKey, err := a.secretManager.HasClaudeAPIKey(ctx)
	if err != nil {
		return scheduler.Result{}, fmt.Errorf("Claude API キー状態を確認できませんでした: %w", err)
	}
	if !hasGoogleToken || !hasClaudeKey {
		return scheduler.Result{
			Skipped: true,
			Message: "Google トークンまたは Claude API キーが未設定のため、自動分類ジョブをスキップしました。",
		}, nil
	}

	token, err := a.secretManager.LoadGoogleToken(ctx)
	if err != nil {
		return scheduler.Result{}, fmt.Errorf("保存済み Google トークンを読み出せませんでした: %w", err)
	}
	token, refreshed, err := a.authClient.EnsureValidToken(ctx, token)
	if err != nil {
		return scheduler.Result{}, fmt.Errorf("Google トークンを再利用できませんでした: %w", err)
	}
	if refreshed {
		if err := a.secretManager.SaveGoogleToken(ctx, token); err != nil {
			return scheduler.Result{}, fmt.Errorf("更新した Google トークンをキーチェーンへ保存できませんでした: %w", err)
		}
	}

	fetchQuery := buildSchedulerClassificationQuery(*lastRunAt)
	labelIDs := []string{"INBOX"}
	checkpoint, hasCheckpoint := a.loadClassificationCheckpoint(
		*lastRunAt,
		fetchQuery,
		labelIDs,
	)
	if !hasCheckpoint {
		checkpoint = newClassificationCheckpoint(*lastRunAt, fetchQuery, labelIDs)
		if err := a.saveClassificationCheckpoint(checkpoint); err != nil {
			return scheduler.Result{}, fmt.Errorf("分類 checkpoint の初期化に失敗しました: %w", err)
		}
	}
	initialCompletedBatches := checkpoint.CompletedBatches
	pageToken := ""
	processedPageCount := 0
	currentProcessedCount := 0
	aggregated := scheduler.Result{
		Processed:       checkpoint.Processed,
		Success:         checkpoint.Success,
		Failed:          checkpoint.Failed,
		PendingApproval: checkpoint.PendingApproval,
	}
	completedBatches := checkpoint.CompletedBatches

	for {
		fetched, err := a.gmailClient.FetchMessages(ctx, token.AccessToken, gmail.FetchRequest{
			MaxResults: types.ClassificationMaxBatchSize,
			LabelIDs:   labelIDs,
			Query:      fetchQuery,
			PageToken:  pageToken,
		})
		if err != nil {
			return aggregated, maybeMarkSchedulerRetryable(
				fmt.Errorf("新着メール取得に失敗しました: %w", err),
			)
		}

		messages := fetched.Messages
		if completedBatches > 0 {
			completedBatches--
			nextPageToken := strings.TrimSpace(fetched.NextPageToken)
			if nextPageToken == "" {
				break
			}
			pageToken = nextPageToken
			continue
		}
		processedPageCount++

		pageResult, err := a.runScheduledClassificationPage(messages)
		if err != nil {
			failed := aggregated
			failed.Processed += pageResult.Processed
			failed.Success += pageResult.Success
			failed.Failed += pageResult.Failed
			failed.PendingApproval += pageResult.PendingApproval
			return failed, maybeMarkSchedulerRetryable(err)
		}

		nextAggregated := aggregated
		nextAggregated.Processed += pageResult.Processed
		nextAggregated.Success += pageResult.Success
		nextAggregated.Failed += pageResult.Failed
		nextAggregated.PendingApproval += pageResult.PendingApproval
		nextCurrentProcessedCount := currentProcessedCount + pageResult.Processed
		nextCheckpoint := checkpoint
		nextCheckpoint.CompletedBatches++
		nextCheckpoint.Processed = nextAggregated.Processed
		nextCheckpoint.Success = nextAggregated.Success
		nextCheckpoint.Failed = nextAggregated.Failed
		nextCheckpoint.PendingApproval = nextAggregated.PendingApproval
		nextCheckpoint.Skipped += schedulerResultSkippedCount(pageResult)
		nextCheckpoint.LastStopReason = ""
		nextCheckpoint.LastError = ""
		nextCheckpoint.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if err := a.saveClassificationCheckpoint(nextCheckpoint); err != nil {
			return aggregated, fmt.Errorf("分類 checkpoint を保存できませんでした: %w", err)
		}
		aggregated = nextAggregated
		currentProcessedCount = nextCurrentProcessedCount
		checkpoint = nextCheckpoint

		nextPageToken := strings.TrimSpace(fetched.NextPageToken)
		if nextPageToken == "" {
			break
		}
		pageToken = nextPageToken
	}

	if currentProcessedCount == 0 && aggregated.Processed == 0 {
		if err := a.clearClassificationCheckpoint(); err != nil {
			log.Printf("分類 checkpoint のクリアに失敗しました: %v", err)
		}
		return scheduler.Result{
			Skipped: true,
			Message: fmt.Sprintf(
				"last_run_at(%s) 以降の新着メールはありませんでした。",
				lastRunAt.Format(time.RFC3339),
			),
		}, nil
	}

	aggregated.Message = buildScheduledClassificationSummary(
		aggregated,
		currentProcessedCount,
		processedPageCount,
		initialCompletedBatches,
	)
	return aggregated, nil
}

func schedulerResultSkippedCount(result scheduler.Result) int {
	skipped := result.Processed - result.Success - result.Failed - result.PendingApproval
	if skipped < 0 {
		return 0
	}
	return skipped
}

func buildScheduledClassificationSummary(
	aggregated scheduler.Result,
	currentProcessedCount int,
	processedPageCount int,
	initialCompletedBatches int,
) string {
	skippedCount := schedulerResultSkippedCount(aggregated)
	if initialCompletedBatches <= 0 {
		return fmt.Sprintf(
			"新着 %d 件を %d ページで処理しました。完了 %d件 / 失敗 %d件 / 承認待ち %d件 / スキップ %d件。",
			currentProcessedCount,
			processedPageCount,
			aggregated.Success,
			aggregated.Failed,
			aggregated.PendingApproval,
			skippedCount,
		)
	}
	if currentProcessedCount == 0 {
		return fmt.Sprintf(
			"checkpoint から再開しましたが、追加処理対象はありませんでした（スキップ %d ページ）。累計: 完了 %d件 / 失敗 %d件 / 承認待ち %d件 / スキップ %d件。",
			initialCompletedBatches,
			aggregated.Success,
			aggregated.Failed,
			aggregated.PendingApproval,
			skippedCount,
		)
	}
	return fmt.Sprintf(
		"checkpoint から再開し、今回 %d 件を %d ページで追加処理しました（スキップ %d ページ）。累計: 完了 %d件 / 失敗 %d件 / 承認待ち %d件 / スキップ %d件。",
		currentProcessedCount,
		processedPageCount,
		initialCompletedBatches,
		aggregated.Success,
		aggregated.Failed,
		aggregated.PendingApproval,
		skippedCount,
	)
}

func (a *App) runScheduledBlocklist(ctx context.Context) (scheduler.Result, error) {
	if a.runScheduledBlocklistJob != nil {
		return a.runScheduledBlocklistJob(ctx)
	}

	store, err := a.requireDBStore()
	if err != nil {
		return scheduler.Result{}, err
	}

	suggestions, err := store.ListBlocklistSuggestions(ctx, blocklistSuggestionMinimum)
	if err != nil {
		return scheduler.Result{}, fmt.Errorf("ブロックリスト提案の取得に失敗しました: %w", err)
	}

	imported := 0
	for _, suggestion := range suggestions {
		_, err := store.UpsertBlocklistEntry(
			ctx,
			suggestion.Kind,
			suggestion.Pattern,
			fmt.Sprintf("scheduler 自動登録: %s", suggestion.Description),
		)
		if err != nil {
			return scheduler.Result{}, fmt.Errorf("ブロックリスト自動更新に失敗しました: %w", err)
		}
		imported++
	}

	if imported == 0 {
		return scheduler.Result{
			Skipped: true,
			Message: "ブロックリスト更新対象はありませんでした。",
		}, nil
	}

	return scheduler.Result{
		Processed: imported,
		Success:   imported,
		Message:   fmt.Sprintf("ブロックリストを %d 件更新しました。", imported),
	}, nil
}

func (a *App) runScheduledKnownBlock(ctx context.Context) (scheduler.Result, error) {
	if a.runScheduledKnownBlockJob != nil {
		return a.runScheduledKnownBlockJob(ctx)
	}

	store, err := a.requireDBStore()
	if err != nil {
		return scheduler.Result{}, err
	}

	entries, err := store.ListBlocklistEntries(ctx)
	if err != nil {
		return scheduler.Result{}, fmt.Errorf("既知ブロック処理の準備に失敗しました: %w", err)
	}
	if len(entries) == 0 {
		return scheduler.Result{
			Skipped: true,
			Message: "ブロックリストが空のため、既知ブロック処理をスキップしました。",
		}, nil
	}

	hasGoogleToken, err := a.secretManager.HasGoogleToken(ctx)
	if err != nil {
		return scheduler.Result{}, fmt.Errorf("Google トークン状態を確認できませんでした: %w", err)
	}
	if !hasGoogleToken {
		return scheduler.Result{
			Skipped: true,
			Message: "Google トークン未設定のため、既知ブロック処理をスキップしました。",
		}, nil
	}

	return scheduler.Result{
		Skipped: true,
		Message: "既知ブロック送信者の自動処理基盤を起動し、次フェーズの Gmail 取得処理に備えました。",
	}, nil
}

func buildSchedulerClassificationQuery(lastRunAt time.Time) string {
	normalized := lastRunAt.UTC()
	if normalized.IsZero() {
		return ""
	}
	return fmt.Sprintf("after:%d", normalized.Unix())
}

func (a *App) runScheduledClassificationPage(messages []types.EmailSummary) (scheduler.Result, error) {
	if len(messages) == 0 {
		return scheduler.Result{}, nil
	}

	classification, err := a.ClassifyEmails(types.ClassificationRequest{
		Messages: messages,
	})
	if err != nil {
		return scheduler.Result{}, fmt.Errorf("新着メール分類に失敗しました: %w", err)
	}
	if len(classification.Results) == 0 {
		return scheduler.Result{}, nil
	}

	decisions, metadata, pendingApproval := buildSchedulerSafeRunDecisions(
		messages,
		classification.Results,
	)

	result := scheduler.Result{
		Processed:       len(classification.Results),
		PendingApproval: pendingApproval,
	}
	if len(decisions) == 0 {
		return result, nil
	}

	actionResult, err := a.ExecuteGmailActions(types.ExecuteGmailActionsRequest{
		Confirmed: true,
		Decisions: decisions,
		Metadata:  metadata,
	})
	if err != nil {
		return result, fmt.Errorf("定期 safe-run 反映に失敗しました: %w", err)
	}

	result.Success = actionResult.SuccessCount
	result.Failed = actionResult.FailureCount
	if actionResult.FailureCount > 0 {
		return result, fmt.Errorf("safe-run 反映に失敗が含まれます: %s", actionResult.Message)
	}
	return result, nil
}

func buildSchedulerSafeRunDecisions(
	messages []types.EmailSummary,
	results []types.ClassificationResult,
) ([]types.GmailActionDecision, []types.GmailActionMetadata, int) {
	messageByID := make(map[string]types.EmailSummary, len(messages))
	for _, message := range messages {
		messageByID[strings.TrimSpace(message.ID)] = message
	}

	decisions := make([]types.GmailActionDecision, 0, len(results))
	metadata := make([]types.GmailActionMetadata, 0, len(results))
	pendingApproval := 0

	for _, result := range results {
		messageID := strings.TrimSpace(result.MessageID)
		if messageID == "" {
			continue
		}
		if result.ReviewLevel != types.ClassificationReviewLevelAutoApply {
			pendingApproval++
			continue
		}

		safeCategory := schedulerSafeRunCategory(result.Category)
		decisions = append(decisions, types.GmailActionDecision{
			MessageID:   messageID,
			Category:    safeCategory,
			ReviewLevel: result.ReviewLevel,
		})

		source := result.Source
		if !source.IsValid() {
			source = types.ClassificationSourceClaude
		}
		message := messageByID[messageID]
		metadata = append(metadata, types.GmailActionMetadata{
			MessageID:   messageID,
			ThreadID:    strings.TrimSpace(message.ThreadID),
			From:        strings.TrimSpace(message.From),
			Subject:     strings.TrimSpace(message.Subject),
			Category:    safeCategory,
			Confidence:  result.Confidence,
			ReviewLevel: result.ReviewLevel,
			Source:      source,
		})
	}

	return decisions, metadata, pendingApproval
}

func schedulerSafeRunCategory(category types.ClassificationCategory) types.ClassificationCategory {
	if category == types.ClassificationCategoryJunk {
		return types.ClassificationCategoryArchive
	}
	return category
}

func maybeMarkSchedulerRetryable(err error) error {
	if err == nil {
		return nil
	}

	if statusCode, ok := schedulerHTTPStatusCode(err); ok {
		if statusCode == 429 || (statusCode >= 500 && statusCode <= 599) {
			return scheduler.MarkRetryable(err)
		}
	}

	text := strings.ToLower(err.Error())
	if strings.Contains(text, "timeout") ||
		strings.Contains(text, "temporarily unavailable") ||
		strings.Contains(text, "connection reset") {
		return scheduler.MarkRetryable(err)
	}

	return err
}

func schedulerHTTPStatusCode(err error) (int, bool) {
	if err == nil {
		return 0, false
	}

	var withStatus interface{ StatusCode() int }
	if errors.As(err, &withStatus) {
		statusCode := withStatus.StatusCode()
		if statusCode >= 100 && statusCode <= 599 {
			return statusCode, true
		}
	}

	text := strings.ToLower(err.Error())
	for statusCode := 100; statusCode <= 599; statusCode++ {
		if strings.Contains(text, fmt.Sprintf("http %d", statusCode)) ||
			strings.Contains(text, fmt.Sprintf("(%d)", statusCode)) {
			return statusCode, true
		}
	}
	return 0, false
}

func (a *App) saveSchedulerLastRun(jobID string, runAt time.Time, updateCommon bool) error {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return errors.New("scheduler job ID が空です")
	}

	store, err := a.requireDBStore()
	if err != nil {
		return err
	}

	if runAt.IsZero() {
		runAt = time.Now().UTC()
	}

	ctx, cancel := context.WithTimeout(a.baseContext(), dbOperationTimeout)
	defer cancel()

	lastRunAt := runAt.UTC().Format(time.RFC3339)
	jobKey := fmt.Sprintf(schedulerSettingLastRunTemplate, jobID)
	if err := store.SetSetting(ctx, jobKey, lastRunAt); err != nil {
		return fmt.Errorf("scheduler ジョブ実行時刻を保存できませんでした: %w", err)
	}
	if !updateCommon {
		return nil
	}
	if err := store.SetSetting(ctx, schedulerSettingLastRunAt, lastRunAt); err != nil {
		return fmt.Errorf("scheduler 最終実行時刻を保存できませんでした: %w", err)
	}
	return nil
}

func (a *App) loadSchedulerLastRunAt() *time.Time {
	store, err := a.requireDBStore()
	if err != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(a.baseContext(), dbOperationTimeout)
	defer cancel()

	value, ok, err := store.GetSetting(ctx, schedulerSettingLastRunAt)
	if err != nil || !ok {
		return nil
	}

	return parseSchedulerRunAt(value)
}

func (a *App) loadSchedulerJobLastRunAt(jobID string) *time.Time {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil
	}

	store, err := a.requireDBStore()
	if err != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(a.baseContext(), dbOperationTimeout)
	defer cancel()

	jobKey := fmt.Sprintf(schedulerSettingLastRunTemplate, jobID)
	value, ok, err := store.GetSetting(ctx, jobKey)
	if err != nil || !ok {
		return nil
	}

	return parseSchedulerRunAt(value)
}

func parseSchedulerRunAt(value string) *time.Time {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return nil
	}
	lastRunAt := parsed.UTC()
	return &lastRunAt
}

func newClassificationCheckpoint(
	lastRunAt time.Time,
	fetchQuery string,
	labelIDs []string,
) classificationCheckpoint {
	normalizedLabels := make([]string, 0, len(labelIDs))
	for _, labelID := range labelIDs {
		trimmed := strings.TrimSpace(labelID)
		if trimmed == "" {
			continue
		}
		normalizedLabels = append(normalizedLabels, trimmed)
	}

	return classificationCheckpoint{
		RunType:   schedulerCheckpointRunTypeScheduled,
		Mode:      schedulerCheckpointModeSafeRun,
		LastRunAt: lastRunAt.UTC().Format(time.RFC3339),
		Query:     strings.TrimSpace(fetchQuery),
		LabelIDs:  normalizedLabels,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

func (a *App) loadClassificationCheckpoint(
	lastRunAt time.Time,
	fetchQuery string,
	labelIDs []string,
) (classificationCheckpoint, bool) {
	store, err := a.requireDBStore()
	if err != nil {
		return classificationCheckpoint{}, false
	}

	ctx, cancel := context.WithTimeout(a.baseContext(), dbOperationTimeout)
	defer cancel()

	value, ok, err := store.GetSetting(ctx, schedulerSettingClassificationCheckpoint)
	if err != nil || !ok {
		return classificationCheckpoint{}, false
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return classificationCheckpoint{}, false
	}

	var checkpoint classificationCheckpoint
	if err := json.Unmarshal([]byte(trimmed), &checkpoint); err != nil {
		log.Printf("分類 checkpoint の形式が不正です: %v", err)
		return classificationCheckpoint{}, false
	}

	if checkpoint.RunType != schedulerCheckpointRunTypeScheduled ||
		checkpoint.Mode != schedulerCheckpointModeSafeRun ||
		checkpoint.Query != strings.TrimSpace(fetchQuery) ||
		checkpoint.LastRunAt != lastRunAt.UTC().Format(time.RFC3339) ||
		!sameStrings(checkpoint.LabelIDs, labelIDs) {
		return classificationCheckpoint{}, false
	}
	if checkpoint.CompletedBatches < 0 ||
		checkpoint.Processed < 0 ||
		checkpoint.Success < 0 ||
		checkpoint.Failed < 0 ||
		checkpoint.PendingApproval < 0 ||
		checkpoint.Skipped < 0 {
		return classificationCheckpoint{}, false
	}
	return checkpoint, true
}

func (a *App) saveClassificationCheckpoint(checkpoint classificationCheckpoint) error {
	store, err := a.requireDBStore()
	if err != nil {
		return err
	}

	data, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("分類 checkpoint の JSON 変換に失敗しました: %w", err)
	}

	ctx, cancel := context.WithTimeout(a.baseContext(), dbOperationTimeout)
	defer cancel()

	if err := store.SetSetting(ctx, schedulerSettingClassificationCheckpoint, string(data)); err != nil {
		return err
	}
	return nil
}

func (a *App) clearClassificationCheckpoint() error {
	store, err := a.requireDBStore()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(a.baseContext(), dbOperationTimeout)
	defer cancel()

	if err := store.SetSetting(ctx, schedulerSettingClassificationCheckpoint, ""); err != nil {
		return err
	}
	return nil
}

func (a *App) updateClassificationCheckpointFailure(event scheduler.Event) error {
	if strings.TrimSpace(event.JobID) != schedulerJobClassification {
		return nil
	}

	store, err := a.requireDBStore()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(a.baseContext(), dbOperationTimeout)
	defer cancel()

	value, ok, err := store.GetSetting(ctx, schedulerSettingClassificationCheckpoint)
	if err != nil || !ok {
		return err
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	var checkpoint classificationCheckpoint
	if err := json.Unmarshal([]byte(trimmed), &checkpoint); err != nil {
		return fmt.Errorf("分類 checkpoint の読み出しに失敗しました: %w", err)
	}

	checkpoint.LastStopReason = schedulerStopReason(event)
	if event.Err != nil {
		checkpoint.LastError = strings.TrimSpace(event.Err.Error())
	}
	checkpoint.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	data, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("分類 checkpoint の JSON 変換に失敗しました: %w", err)
	}
	if err := store.SetSetting(ctx, schedulerSettingClassificationCheckpoint, string(data)); err != nil {
		return err
	}
	return nil
}

func sameStrings(left []string, right []string) bool {
	leftNormalized := normalizedStrings(left)
	rightNormalized := normalizedStrings(right)
	if len(leftNormalized) != len(rightNormalized) {
		return false
	}
	for index := range leftNormalized {
		if leftNormalized[index] != rightNormalized[index] {
			return false
		}
	}
	return true
}

func normalizedStrings(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	return normalized
}

func (a *App) logSchedulerEvent(event scheduler.Event) {
	switch event.Kind {
	case scheduler.EventKindStarted:
		log.Printf("[scheduler] job=%s attempt=%d started", event.JobID, event.Attempt)
	case scheduler.EventKindRetryScheduled:
		log.Printf(
			"[scheduler] job=%s attempt=%d retry_in=%s err=%v",
			event.JobID,
			event.Attempt,
			event.Delay,
			event.Err,
		)
	case scheduler.EventKindSucceeded:
		if strings.TrimSpace(event.JobID) == schedulerJobClassification {
			if err := a.clearClassificationCheckpoint(); err != nil {
				log.Printf("[scheduler] job=%s checkpoint クリアに失敗しました: %v", event.JobID, err)
			}
		}
		if err := a.saveSchedulerLastRun(
			event.JobID,
			event.At,
			event.JobID == schedulerJobClassification,
		); err != nil {
			log.Printf("[scheduler] job=%s last_run 保存に失敗しました: %v", event.JobID, err)
		}
		log.Printf(
			"[scheduler] job=%s succeeded processed=%d success=%d failed=%d pending=%d skipped=%d message=%s",
			event.JobID,
			event.Result.Processed,
			event.Result.Success,
			event.Result.Failed,
			event.Result.PendingApproval,
			schedulerResultSkippedCount(event.Result),
			event.Result.Message,
		)
		a.emitSchedulerNotification(event)
	case scheduler.EventKindSkipped:
		log.Printf("[scheduler] job=%s skipped message=%s", event.JobID, event.Result.Message)
		a.emitSchedulerNotification(event)
	case scheduler.EventKindOverlapSkipped:
		log.Printf("[scheduler] job=%s skipped because previous run is still active", event.JobID)
	case scheduler.EventKindFailed:
		if strings.TrimSpace(event.JobID) == schedulerJobClassification {
			if err := a.updateClassificationCheckpointFailure(event); err != nil {
				log.Printf("[scheduler] job=%s checkpoint 更新に失敗しました: %v", event.JobID, err)
			}
		}
		log.Printf(
			"[scheduler] job=%s failed attempt=%d/%d err=%v",
			event.JobID,
			event.Attempt,
			event.MaxRetries+1,
			event.Err,
		)
		a.emitSchedulerNotification(event)
	}
}

func (a *App) emitSchedulerNotification(event scheduler.Event) {
	if event.Kind != scheduler.EventKindSucceeded &&
		event.Kind != scheduler.EventKindSkipped &&
		event.Kind != scheduler.EventKindFailed {
		return
	}
	if a.eventsEmit == nil || a.ctx == nil {
		return
	}

	store, err := a.requireDBStore()
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(a.baseContext(), dbOperationTimeout)
	defer cancel()

	if !a.loadSchedulerNotificationsEnabled(ctx, store) {
		return
	}

	notification, ok := buildSchedulerNotification(event)
	if !ok {
		return
	}
	a.eventsEmit(a.ctx, schedulerNotificationEventName, notification)
}

func buildSchedulerNotification(event scheduler.Event) (types.SchedulerNotification, bool) {
	at := event.At.UTC()
	if at.IsZero() {
		at = time.Now().UTC()
	}

	jobName := schedulerJobDisplayName(event.JobID)
	summary := fmt.Sprintf(
		"完了 %d件 / 失敗 %d件 / 承認待ち %d件 / スキップ %d件",
		event.Result.Success,
		event.Result.Failed,
		event.Result.PendingApproval,
		schedulerResultSkippedCount(event.Result),
	)

	switch event.Kind {
	case scheduler.EventKindSucceeded:
		level := "info"
		if event.Result.Failed > 0 || event.Result.PendingApproval > 0 {
			level = "warning"
		}

		body := summary
		if message := strings.TrimSpace(event.Result.Message); message != "" {
			body = fmt.Sprintf("%s / %s", summary, message)
		}
		return types.SchedulerNotification{
			Title: fmt.Sprintf("%sが完了しました", jobName),
			Body:  body,
			Level: level,
			JobID: event.JobID,
			At:    at.Format(time.RFC3339),
		}, true
	case scheduler.EventKindSkipped:
		if !schedulerSkipNeedsNotification(event) {
			return types.SchedulerNotification{}, false
		}

		body := strings.TrimSpace(event.Result.Message)
		if body == "" {
			body = "実行条件を満たさないため、処理をスキップしました。"
		}
		if schedulerNeedsSettingsGuidance(event.Result.Message, event.Err) {
			body += " Settings 画面で Google 認証と Claude API キーを確認してください。"
		}

		return types.SchedulerNotification{
			Title: fmt.Sprintf("%sをスキップしました", jobName),
			Body:  body,
			Level: "warning",
			JobID: event.JobID,
			At:    at.Format(time.RFC3339),
		}, true
	case scheduler.EventKindFailed:
		detail := "処理に失敗しました。"
		if event.Err != nil {
			trimmedError := strings.TrimSpace(event.Err.Error())
			if trimmedError != "" {
				detail = trimmedError
			}
		}
		body := fmt.Sprintf("%s / 停止理由: %s / %s", summary, schedulerStopReason(event), detail)
		if schedulerNeedsSettingsGuidance(event.Result.Message, event.Err) {
			body += " Settings 画面で Google 認証と Claude API キーを確認してください。"
		}
		return types.SchedulerNotification{
			Title: fmt.Sprintf("%sでエラーが発生しました", jobName),
			Body:  body,
			Level: "error",
			JobID: event.JobID,
			At:    at.Format(time.RFC3339),
		}, true
	default:
		return types.SchedulerNotification{}, false
	}
}

func schedulerSkipNeedsNotification(event scheduler.Event) bool {
	if event.Result.Failed > 0 || event.Result.PendingApproval > 0 {
		return true
	}
	if schedulerNeedsSettingsGuidance(event.Result.Message, event.Err) {
		return true
	}
	return schedulerNeedsManualRunGuidance(event.Result.Message)
}

func schedulerNeedsManualRunGuidance(message string) bool {
	text := strings.ToLower(strings.TrimSpace(message))
	return strings.Contains(text, "last_run_at") && strings.Contains(text, "手動実行")
}

func schedulerStopReason(event scheduler.Event) string {
	switch {
	case errors.Is(event.Err, context.DeadlineExceeded):
		return "timeout (時間超過)"
	case errors.Is(event.Err, context.Canceled):
		return "canceled (キャンセル)"
	case schedulerNeedsSettingsGuidance(event.Result.Message, event.Err):
		return "auth_or_config_error (認証・設定エラー)"
	case event.Attempt > event.MaxRetries:
		return "retry_exhausted (再試行上限)"
	default:
		return "runtime_error (実行エラー)"
	}
}

func schedulerNeedsSettingsGuidance(message string, runErr error) bool {
	parts := []string{strings.ToLower(strings.TrimSpace(message))}
	if runErr != nil {
		parts = append(parts, strings.ToLower(runErr.Error()))
	}
	text := strings.Join(parts, " ")

	keywords := []string{
		"google トークン",
		"google token",
		"access token",
		"refresh token",
		"refresh_token",
		"token expired",
		"token has expired",
		"claude api キー",
		"claude api key",
		"認証",
		"unauthorized",
		"forbidden",
		"invalid_grant",
		"invalid api key",
	}
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func schedulerJobDisplayName(jobID string) string {
	switch strings.TrimSpace(jobID) {
	case schedulerJobClassification:
		return "メール分類ジョブ"
	case schedulerJobBlocklist:
		return "ブロックリスト更新ジョブ"
	case schedulerJobKnownBlock:
		return "既知ブロック処理ジョブ"
	default:
		return "定期実行ジョブ"
	}
}

func (a *App) exportActionLogs(
	title string,
	prefix string,
	extension string,
	filters []runtime.FileFilter,
	build func([]types.ActionLogEntry, time.Time) ([]byte, error),
) types.OperationResult {
	store, err := a.requireDBStore()
	if err != nil {
		return types.OperationResult{Success: false, Message: err.Error()}
	}

	ctx, cancel := context.WithTimeout(a.baseContext(), dbOperationTimeout)
	defer cancel()

	entries, err := store.ListActionLogEntries(ctx)
	if err != nil {
		return types.OperationResult{
			Success: false,
			Message: fmt.Sprintf("処理ログを取得できませんでした: %v", err),
		}
	}

	exportedAt := time.Now()
	data, err := build(entries, exportedAt)
	if err != nil {
		return types.OperationResult{
			Success: false,
			Message: fmt.Sprintf("エクスポートデータを生成できませんでした: %v", err),
		}
	}

	return a.saveExportFile(title, buildExportFilename(prefix, exportedAt, extension), filters, data)
}

func (a *App) exportClassificationLogs(
	title string,
	prefix string,
	extension string,
	filters []runtime.FileFilter,
	build func([]types.ClassificationLogEntry, time.Time) ([]byte, error),
) types.OperationResult {
	store, err := a.requireDBStore()
	if err != nil {
		return types.OperationResult{Success: false, Message: err.Error()}
	}

	ctx, cancel := context.WithTimeout(a.baseContext(), dbOperationTimeout)
	defer cancel()

	entries, err := store.ListClassificationLogEntries(ctx)
	if err != nil {
		return types.OperationResult{
			Success: false,
			Message: fmt.Sprintf("分類ログを取得できませんでした: %v", err),
		}
	}

	exportedAt := time.Now()
	data, err := build(entries, exportedAt)
	if err != nil {
		return types.OperationResult{
			Success: false,
			Message: fmt.Sprintf("エクスポートデータを生成できませんでした: %v", err),
		}
	}

	return a.saveExportFile(title, buildExportFilename(prefix, exportedAt, extension), filters, data)
}

func (a *App) saveExportFile(
	title string,
	filename string,
	filters []runtime.FileFilter,
	data []byte,
) types.OperationResult {
	path, err := runtime.SaveFileDialog(a.baseContext(), runtime.SaveDialogOptions{
		Title:            title,
		DefaultDirectory: defaultExportDirectory(),
		DefaultFilename:  filename,
		Filters:          filters,
	})
	if err != nil {
		return types.OperationResult{
			Success: false,
			Message: fmt.Sprintf("保存ダイアログを開けませんでした: %v", err),
		}
	}
	if strings.TrimSpace(path) == "" {
		return types.OperationResult{
			Success: false,
			Message: "ファイル保存をキャンセルしました。",
		}
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return types.OperationResult{
			Success: false,
			Message: fmt.Sprintf("ファイルを書き込めませんでした: %v", err),
		}
	}

	return types.OperationResult{
		Success: true,
		Message: fmt.Sprintf("エクスポートを保存しました: %s", filepath.Base(path)),
	}
}

func buildClassificationLogEntries(
	request types.RecordClassificationRunRequest,
) ([]types.ClassificationLogEntry, error) {
	if len(request.Messages) == 0 {
		return nil, errors.New("分類対象メールがありません")
	}
	if len(request.Results) == 0 {
		return nil, errors.New("分類結果がありません")
	}

	messageByID := make(map[string]types.EmailSummary, len(request.Messages))
	for _, message := range request.Messages {
		messageByID[strings.TrimSpace(message.ID)] = message
	}

	entries := make([]types.ClassificationLogEntry, 0, len(request.Results))
	for _, result := range request.Results {
		messageID := strings.TrimSpace(result.MessageID)
		if messageID == "" {
			return nil, errors.New("分類結果の message_id が空です")
		}
		if !result.Category.IsValid() {
			return nil, fmt.Errorf("分類結果 %q の category が不正です", messageID)
		}
		if !result.ReviewLevel.IsValid() {
			return nil, fmt.Errorf("分類結果 %q の review_level が不正です", messageID)
		}
		if !result.Source.IsValid() {
			return nil, fmt.Errorf("分類結果 %q の source が不正です", messageID)
		}

		message, ok := messageByID[messageID]
		if !ok {
			return nil, fmt.Errorf("分類結果 %q に対応するメール情報が見つかりません", messageID)
		}

		entries = append(entries, types.ClassificationLogEntry{
			MessageID:   messageID,
			ThreadID:    strings.TrimSpace(message.ThreadID),
			From:        strings.TrimSpace(message.From),
			Subject:     strings.TrimSpace(message.Subject),
			Snippet:     strings.TrimSpace(message.Snippet),
			Category:    result.Category,
			Confidence:  result.Confidence,
			ReviewLevel: result.ReviewLevel,
			Source:      result.Source,
		})
	}

	return entries, nil
}

func buildActionLogEntries(
	request types.ExecuteGmailActionsRequest,
	result types.ExecuteGmailActionsResult,
) ([]types.ActionLogEntry, error) {
	metadataByID, err := buildActionMetadataMap(request.Metadata)
	if err != nil {
		return nil, err
	}

	failureByID := make(map[string]types.GmailActionFailure, len(result.Failures))
	for _, failure := range result.Failures {
		failureByID[strings.TrimSpace(failure.MessageID)] = failure
	}

	entries := make([]types.ActionLogEntry, 0, len(request.Decisions))
	for _, decision := range request.Decisions {
		messageID := strings.TrimSpace(decision.MessageID)
		entry := buildActionLogEntry(decision, metadataByID[messageID], actionLogStatusSuccess, "")

		if failure, failed := failureByID[messageID]; failed {
			entry.ActionKind = failure.Action
			entry.Status = actionLogStatusFailed
			entry.Detail = strings.TrimSpace(failure.Error)
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

func (a *App) excludeAlreadySucceededActions(
	ctx context.Context,
	store *db.Store,
	request types.ExecuteGmailActionsRequest,
) (types.ExecuteGmailActionsRequest, []types.ActionLogEntry, error) {
	metadataByID, err := buildActionMetadataMap(request.Metadata)
	if err != nil {
		return types.ExecuteGmailActionsRequest{}, nil, err
	}

	filteredDecisions := make([]types.GmailActionDecision, 0, len(request.Decisions))
	filteredMetadata := make([]types.GmailActionMetadata, 0, len(request.Metadata))
	skippedEntries := make([]types.ActionLogEntry, 0)

	for _, decision := range request.Decisions {
		messageID := strings.TrimSpace(decision.MessageID)
		actionKind := primaryActionKindForDecision(decision)
		latest, found, err := store.GetLatestActionLogEntry(ctx, messageID, actionKind)
		if err != nil {
			return types.ExecuteGmailActionsRequest{}, nil, err
		}
		if found {
			status := normalizedActionLogStatus(latest.Status)
			if status == actionLogStatusSuccess {
				log.Printf(
					"Gmail アクション重複防止: message_id=%s action=%s status=success のためスキップ",
					messageID,
					actionKind,
				)
				skippedEntries = append(
					skippedEntries,
					buildActionLogEntry(decision, metadataByID[messageID], actionLogStatusSuccess, actionLogDetailDuplicateSkip),
				)
				continue
			}
			if status == actionLogStatusFailed || status == actionLogStatusPending {
				log.Printf(
					"Gmail アクション再試行: message_id=%s action=%s status=%s のため再実行",
					messageID,
					actionKind,
					status,
				)
			}
		}

		filteredDecisions = append(filteredDecisions, decision)
		if metadata, ok := metadataByID[messageID]; ok {
			filteredMetadata = append(filteredMetadata, metadata)
		}
	}

	return types.ExecuteGmailActionsRequest{
		Confirmed: request.Confirmed,
		Decisions: filteredDecisions,
		Metadata:  filteredMetadata,
	}, skippedEntries, nil
}

func mergeGmailActionSkippedResult(
	result types.ExecuteGmailActionsResult,
	totalDecisions int,
	skippedCount int,
) types.ExecuteGmailActionsResult {
	if totalDecisions > 0 {
		result.ProcessedCount = totalDecisions
	}
	result.SkippedCount = skippedCount
	if skippedCount <= 0 {
		return result
	}

	baseMessage := strings.TrimSpace(result.Message)
	if result.SuccessCount == 0 && result.FailureCount == 0 {
		result.Success = true
		result.Message = fmt.Sprintf("Gmail アクション %d 件を重複防止のためスキップしました。", skippedCount)
		return result
	}

	if baseMessage == "" {
		baseMessage = "Gmail アクションを実行しました。"
	}
	result.Message = fmt.Sprintf("%s スキップ %d 件（既存 success を検出）。", baseMessage, skippedCount)
	return result
}

func buildActionMetadataMap(
	metadata []types.GmailActionMetadata,
) (map[string]types.GmailActionMetadata, error) {
	metadataByID := make(map[string]types.GmailActionMetadata, len(metadata))
	for _, item := range metadata {
		messageID := strings.TrimSpace(item.MessageID)
		if messageID == "" {
			return nil, errors.New("action metadata の message_id が空です")
		}
		metadataByID[messageID] = item
	}
	return metadataByID, nil
}

func buildActionLogEntry(
	decision types.GmailActionDecision,
	metadata types.GmailActionMetadata,
	status string,
	detail string,
) types.ActionLogEntry {
	category := metadata.Category
	if !category.IsValid() {
		category = decision.Category
	}
	reviewLevel := metadata.ReviewLevel
	if !reviewLevel.IsValid() {
		reviewLevel = decision.ReviewLevel
	}
	source := metadata.Source
	if !source.IsValid() {
		source = types.ClassificationSourceClaude
	}

	return types.ActionLogEntry{
		MessageID:   strings.TrimSpace(decision.MessageID),
		ThreadID:    strings.TrimSpace(metadata.ThreadID),
		From:        strings.TrimSpace(metadata.From),
		Subject:     strings.TrimSpace(metadata.Subject),
		ActionKind:  primaryActionKindForDecision(decision),
		Status:      actionLogStatusOrSuccess(status),
		Detail:      strings.TrimSpace(detail),
		Category:    category,
		Confidence:  metadata.Confidence,
		ReviewLevel: reviewLevel,
		Source:      source,
	}
}

func normalizedActionLogStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}

func actionLogStatusOrSuccess(status string) string {
	normalized := strings.ToLower(strings.TrimSpace(status))
	switch normalized {
	case actionLogStatusPending, actionLogStatusFailed:
		return normalized
	default:
		return actionLogStatusSuccess
	}
}

func buildExportFilename(prefix string, now time.Time, extension string) string {
	return fmt.Sprintf("%s-%s%s", prefix, now.Format("20060102-150405"), extension)
}

func defaultExportDirectory() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}

	downloads := filepath.Join(home, defaultExportDirName)
	if info, statErr := os.Stat(downloads); statErr == nil && info.IsDir() {
		return downloads
	}

	return home
}

func primaryActionKindForDecision(decision types.GmailActionDecision) types.ActionKind {
	switch decision.Category {
	case types.ClassificationCategoryJunk:
		return types.ActionKindDelete
	case types.ClassificationCategoryArchive:
		return types.ActionKindArchive
	case types.ClassificationCategoryNewsletter:
		return types.ActionKindMarkRead
	default:
		return types.ActionKindLabel
	}
}

func (a *App) classifyByBlocklist(
	ctx context.Context,
	messages []types.EmailSummary,
) ([]types.EmailSummary, []types.ClassificationResult, error) {
	store, err := a.requireDBStore()
	if err != nil {
		// DB 未初期化時は従来どおり全件を Claude 分類対象にする。
		return messages, nil, nil
	}

	entries, err := store.ListBlocklistEntries(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("ブロックリスト取得に失敗しました: %w", err)
	}
	if len(entries) == 0 {
		return messages, nil, nil
	}

	senderSet := make(map[string]struct{})
	domainSet := make(map[string]struct{})
	for _, entry := range entries {
		switch entry.Kind {
		case types.BlocklistKindSender:
			if normalized := types.NormalizeSenderAddress(entry.Pattern); normalized != "" {
				senderSet[normalized] = struct{}{}
			}
		case types.BlocklistKindDomain:
			if normalized := normalizeDomain(entry.Pattern); normalized != "" {
				domainSet[normalized] = struct{}{}
			}
		}
	}

	unblocked := make([]types.EmailSummary, 0, len(messages))
	skipped := make([]types.ClassificationResult, 0)
	for _, message := range messages {
		sender, domain := senderIdentity(message.From)
		if sender == "" && domain == "" {
			unblocked = append(unblocked, message)
			continue
		}

		if _, found := senderSet[sender]; found {
			skipped = append(skipped, blocklistClassificationResult(message.ID, "sender", sender))
			continue
		}
		if _, found := domainSet[domain]; found {
			skipped = append(skipped, blocklistClassificationResult(message.ID, "domain", domain))
			continue
		}

		unblocked = append(unblocked, message)
	}

	return unblocked, skipped, nil
}

func mergeClassificationResults(
	messages []types.EmailSummary,
	claudeResults []types.ClassificationResult,
	blocklistResults []types.ClassificationResult,
) []types.ClassificationResult {
	byID := make(map[string]types.ClassificationResult, len(claudeResults)+len(blocklistResults))
	for _, result := range claudeResults {
		byID[result.MessageID] = result
	}
	for _, result := range blocklistResults {
		byID[result.MessageID] = result
	}

	merged := make([]types.ClassificationResult, 0, len(byID))
	for _, message := range messages {
		result, ok := byID[message.ID]
		if !ok {
			continue
		}
		merged = append(merged, result)
	}

	return merged
}

func blocklistClassificationResult(
	messageID string,
	reasonType string,
	reasonValue string,
) types.ClassificationResult {
	return types.ClassificationResult{
		MessageID:   messageID,
		Category:    types.ClassificationCategoryJunk,
		Confidence:  1,
		Reason:      fmt.Sprintf("ブロックリスト一致 (%s: %s) のため Claude 分析をスキップしました。", reasonType, reasonValue),
		ReviewLevel: types.ClassificationReviewLevelAutoApply,
		Source:      types.ClassificationSourceBlocklist,
	}
}

func senderIdentity(raw string) (sender string, domain string) {
	sender = types.NormalizeSenderAddress(raw)
	domain = types.SenderDomain(sender)
	return sender, domain
}

func normalizeDomain(raw string) string {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	if trimmed == "" {
		return ""
	}
	trimmed = strings.TrimPrefix(trimmed, "@")
	if at := strings.LastIndex(trimmed, "@"); at >= 0 {
		trimmed = trimmed[at+1:]
	}
	if strings.Contains(trimmed, " ") {
		return ""
	}
	return trimmed
}

func (a *App) initialAuthStatus() string {
	if !a.authClient.IsConfigured() {
		return buildAuthStatusMessage(false)
	}

	stored, err := a.secretManager.HasGoogleToken(context.Background())
	if err != nil {
		return buildCredentialErrorMessage("Google 認証状態を確認できません。", err)
	}
	if stored {
		return buildStoredAuthStatusMessage()
	}

	return buildAuthStatusMessage(true)
}

func (a *App) initialClaudeStatus() string {
	stored, err := a.secretManager.HasClaudeAPIKey(context.Background())
	if err != nil {
		return buildCredentialErrorMessage("Claude API キー状態を確認できません。", err)
	}
	if stored {
		return buildStoredClaudeStatusMessage()
	}

	return buildUnstoredClaudeStatusMessage()
}

func (a *App) initialGmailStatus() string {
	stored, err := a.secretManager.HasGoogleToken(context.Background())
	if err != nil {
		return buildCredentialErrorMessage("Gmail 接続確認の準備状態を確認できません。", err)
	}
	if stored {
		return buildReadyGmailStatusMessage()
	}

	return buildBlockedGmailStatusMessage()
}
