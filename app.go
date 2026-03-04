package main

import (
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"sync"

	"mairu/internal/auth"
	"mairu/internal/db"
	"mairu/internal/types"
)

type App struct {
	ctx           context.Context
	authClient    *auth.Client
	secretManager *auth.SecretManager
	dbStore       *db.Store

	mu             sync.RWMutex
	authStatus     string
	claudeStatus   string
	databaseReady  bool
	loginCancel    context.CancelFunc
	loginCancelSeq uint64
}

func NewApp() *App {
	clientID := strings.TrimSpace(os.Getenv("MAIRU_GOOGLE_OAUTH_CLIENT_ID"))
	clientSecret := strings.TrimSpace(os.Getenv("MAIRU_GOOGLE_OAUTH_CLIENT_SECRET"))
	secretManager := auth.NewSecretManager(auth.NewSystemSecretStore())

	app := &App{
		authClient: auth.NewClient(auth.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
		}),
		secretManager: secretManager,
	}
	app.authStatus = app.initialAuthStatus()
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
	claudeStatus := a.claudeStatus
	databaseReady := a.databaseReady
	a.mu.RUnlock()

	baseContext := a.baseContext()
	googleConfigured := a.authClient.IsConfigured()

	authorized, err := a.secretManager.HasGoogleToken(baseContext)
	if err != nil {
		authorized = false
		authStatus = buildCredentialErrorMessage("Google 認証状態を確認できません。", err)
	} else if authorized && shouldUseStoredAuthMessage(authStatus) {
		authStatus = buildStoredAuthStatusMessage()
	} else if !authorized && shouldUseUnstoredAuthMessage(authStatus) {
		authStatus = buildAuthStatusMessage(googleConfigured)
	}

	claudeConfigured, err := a.secretManager.HasClaudeAPIKey(baseContext)
	if err != nil {
		claudeConfigured = false
		claudeStatus = buildCredentialErrorMessage("Claude API キー状態を確認できません。", err)
	} else if claudeConfigured && shouldUseStoredClaudeMessage(claudeStatus) {
		claudeStatus = buildStoredClaudeStatusMessage()
	} else if !claudeConfigured && shouldUseUnstoredClaudeMessage(claudeStatus) {
		claudeStatus = buildUnstoredClaudeStatusMessage()
	}

	return types.RuntimeStatus{
		Authorized:       authorized,
		GoogleConfigured: googleConfigured,
		AuthStatus:       authStatus,
		ClaudeConfigured: claudeConfigured,
		ClaudeStatus:     claudeStatus,
		DatabaseReady:    databaseReady,
		LastRunAt:        nil,
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
	return "Google トークンをキーチェーンに保存しました。次の issue で Gmail 接続確認へ進めます。"
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
