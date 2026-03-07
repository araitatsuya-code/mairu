package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/mail"
	"os"
	"strings"
	"sync"
	"time"

	"mairu/internal/auth"
	"mairu/internal/claude"
	"mairu/internal/db"
	"mairu/internal/gmail"
	"mairu/internal/types"
)

const (
	gmailConnectionTimeout      = 15 * time.Second
	gmailActionTimeout          = 45 * time.Second
	claudeClassificationTimeout = 45 * time.Second
	dbOperationTimeout          = 10 * time.Second
	blocklistSuggestionMinimum  = 3
)

type App struct {
	ctx           context.Context
	authClient    *auth.Client
	claudeClient  *claude.Client
	gmailClient   *gmail.Client
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
		secretManager: secretManager,
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
	}
}

func (a *App) shutdown(context.Context) {
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

	baseContext := a.baseContext()
	googleConfigured := a.authClient.IsConfigured()
	googleTokenPreview := ""
	claudeKeyPreview := ""

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
		DatabaseReady:      databaseReady,
		LastRunAt:          nil,
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

	result, err := a.gmailClient.ExecuteActions(baseContext, token.AccessToken, request.Decisions)
	if err != nil {
		return types.ExecuteGmailActionsResult{}, err
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
		if response.Results[index].Source == "" {
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

func buildCredentialErrorMessage(prefix string, err error) string {
	log.Printf("%s detail=%v", prefix, err)
	return prefix + " 詳細はアプリのログを確認してください。"
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
		return nil, errors.New("SQLite が初期化されていないためブロックリスト操作を実行できません")
	}

	return store, nil
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
			if normalized := normalizeSenderAddress(entry.Pattern); normalized != "" {
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
	sender = normalizeSenderAddress(raw)
	domain = senderDomain(sender)
	return sender, domain
}

func normalizeSenderAddress(raw string) string {
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

func senderDomain(sender string) string {
	address := normalizeSenderAddress(sender)
	at := strings.LastIndex(address, "@")
	if at < 0 || at+1 >= len(address) {
		return ""
	}
	return address[at+1:]
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
