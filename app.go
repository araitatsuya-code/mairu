package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"

	"mairu/internal/auth"
	"mairu/internal/types"
)

type App struct {
	ctx        context.Context
	authClient *auth.Client

	mu               sync.RWMutex
	authCodeReady    bool
	authCode         string
	authCodeVerifier string
	authStatus       string
	loginCancel      context.CancelFunc
	loginCancelSeq   uint64
}

func NewApp() *App {
	clientID := strings.TrimSpace(os.Getenv("MAIRU_GOOGLE_OAUTH_CLIENT_ID"))

	return &App{
		authClient: auth.NewClient(auth.Config{
			ClientID: clientID,
		}),
		authStatus: buildAuthStatusMessage(clientID != ""),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) AppName() string {
	return "Mairu"
}

// GetRuntimeStatus は起動時に必要な初期状態を返す。
func (a *App) GetRuntimeStatus() types.RuntimeStatus {
	a.mu.RLock()
	authCodeReady := a.authCodeReady
	authStatus := a.authStatus
	a.mu.RUnlock()

	return types.RuntimeStatus{
		Authorized:       authCodeReady,
		GoogleConfigured: a.authClient.IsConfigured(),
		AuthStatus:       authStatus,
		ClaudeConfigured: false,
		DatabaseReady:    false,
		LastRunAt:        nil,
	}
}

// StartGoogleLogin は Google OAuth PKCE ログインを開始し、認可コード受信まで待機する。
func (a *App) StartGoogleLogin() (types.GoogleLoginResult, error) {
	if !a.authClient.IsConfigured() {
		a.setAuthState(false, "", "", buildAuthStatusMessage(false))
		return types.GoogleLoginResult{
			Success: false,
			Message: buildAuthStatusMessage(false),
			Scopes:  a.authClient.Scopes(),
		}, nil
	}

	a.setAuthState(false, "", "", "ブラウザを開いて Google ログインを待っています。")

	loginContext := a.ctx
	if loginContext == nil {
		loginContext = context.Background()
	}

	loginContext, cancel := context.WithCancel(loginContext)
	cancelSeq := a.setLoginCancel(cancel)
	defer a.clearLoginCancel(cancelSeq)

	loginResult, err := a.authClient.RunLoginFlow(loginContext)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			message := "ログイン処理を中断しました。再試行できます。"
			a.setAuthState(false, "", "", message)
			return types.GoogleLoginResult{
				Success: false,
				Message: message,
				Scopes:  a.authClient.Scopes(),
			}, nil
		}

		a.setAuthState(false, "", "", err.Error())
		return types.GoogleLoginResult{}, err
	}

	message := "認可コードを受信しました。次の実装でトークン交換へ進みます。"
	a.setAuthState(true, loginResult.Code, loginResult.CodeVerifier, message)

	return types.GoogleLoginResult{
		Success:          true,
		Message:          message,
		AuthorizationURL: loginResult.AuthorizationURL,
		RedirectURL:      loginResult.RedirectURL,
		CodePreview:      previewCode(loginResult.Code),
		Scopes:           loginResult.Scopes,
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
	a.setAuthState(false, "", "", "ログイン処理を中断しました。再試行できます。")
	return true
}

func (a *App) setAuthState(codeReady bool, authCode string, codeVerifier string, status string) {
	a.mu.Lock()
	a.authCodeReady = codeReady
	a.authCode = authCode
	a.authCodeVerifier = codeVerifier
	a.authStatus = status
	a.mu.Unlock()
}

func (a *App) setLoginCancel(cancel context.CancelFunc) uint64 {
	a.mu.Lock()
	a.loginCancelSeq++
	a.loginCancel = cancel
	seq := a.loginCancelSeq
	a.mu.Unlock()
	return seq
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
		return "環境変数 MAIRU_GOOGLE_OAUTH_CLIENT_ID を設定すると Google ログインを開始できます。"
	}

	return "Google ログインを開始すると、localhost で認可コードを受け取ります。"
}

func previewCode(code string) string {
	if len(code) <= 12 {
		return code
	}

	return code[:12] + "..."
}
