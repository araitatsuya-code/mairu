package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/pkg/browser"
)

const (
	defaultListenHost        = "127.0.0.1"
	defaultCallbackPath      = "/oauth2/callback"
	defaultAuthorizationURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	defaultLoginFlowTimeout  = 2 * time.Minute
	callbackShutdownTimeout  = 5 * time.Second
	codeVerifierEntropyBytes = 64
)

var defaultScopes = []string{
	"https://www.googleapis.com/auth/gmail.modify",
	"https://www.googleapis.com/auth/gmail.labels",
	"openid",
	"email",
	"profile",
}

// Config は Google OAuth クライアントの設定値を表す。
type Config struct {
	ClientID         string
	ListenHost       string
	CallbackPath     string
	AuthorizationURL string
	Scopes           []string
	FlowTimeout      time.Duration
}

// Client は Google OAuth PKCE フローを扱う。
type Client struct {
	clientID         string
	listenHost       string
	callbackPath     string
	authorizationURL string
	scopes           []string
	flowTimeout      time.Duration

	mu      sync.Mutex
	running bool
}

// LoginSession は認可画面へ移動するための一時情報を表す。
type LoginSession struct {
	State            string
	AuthorizationURL string
	RedirectURL      string
	Scopes           []string
}

// LoginResult は認可コード受信後に保持する情報を表す。
type LoginResult struct {
	LoginSession
	Code         string
	CodeVerifier string
}

type callbackResult struct {
	code string
	err  error
}

// NewClient は Google OAuth 用クライアントを初期化する。
func NewClient(config Config) *Client {
	listenHost := strings.TrimSpace(config.ListenHost)
	if listenHost == "" {
		listenHost = defaultListenHost
	}

	callbackPath := strings.TrimSpace(config.CallbackPath)
	if callbackPath == "" {
		callbackPath = defaultCallbackPath
	}
	if !strings.HasPrefix(callbackPath, "/") {
		callbackPath = "/" + callbackPath
	}
	callbackPath = path.Clean(callbackPath)
	if callbackPath == "." {
		callbackPath = defaultCallbackPath
	}

	authorizationURL := strings.TrimSpace(config.AuthorizationURL)
	if authorizationURL == "" {
		authorizationURL = defaultAuthorizationURL
	}

	scopes := cloneStrings(config.Scopes)
	if len(scopes) == 0 {
		scopes = cloneStrings(defaultScopes)
	}

	flowTimeout := config.FlowTimeout
	if flowTimeout <= 0 {
		flowTimeout = defaultLoginFlowTimeout
	}

	return &Client{
		clientID:         strings.TrimSpace(config.ClientID),
		listenHost:       listenHost,
		callbackPath:     callbackPath,
		authorizationURL: authorizationURL,
		scopes:           scopes,
		flowTimeout:      flowTimeout,
	}
}

// IsConfigured はログイン開始に必要な必須設定がそろっているか返す。
func (c *Client) IsConfigured() bool {
	return strings.TrimSpace(c.clientID) != ""
}

// Scopes は現在の OAuth スコープ定義を返す。
func (c *Client) Scopes() []string {
	return cloneStrings(c.scopes)
}

// RunLoginFlow はブラウザを開き、認可コード受信まで待機する。
func (c *Client) RunLoginFlow(ctx context.Context) (LoginResult, error) {
	if !c.IsConfigured() {
		return LoginResult{}, errors.New("Google OAuth クライアント ID が未設定です")
	}

	if err := c.begin(); err != nil {
		return LoginResult{}, err
	}
	defer c.finish()

	state, err := randomToken(32)
	if err != nil {
		return LoginResult{}, fmt.Errorf("state 生成に失敗しました: %w", err)
	}

	codeVerifier, err := randomToken(codeVerifierEntropyBytes)
	if err != nil {
		return LoginResult{}, fmt.Errorf("code verifier 生成に失敗しました: %w", err)
	}
	codeChallenge := buildCodeChallenge(codeVerifier)

	listener, err := net.Listen("tcp", net.JoinHostPort(c.listenHost, "0"))
	if err != nil {
		return LoginResult{}, fmt.Errorf("localhost リダイレクト待受の開始に失敗しました: %w", err)
	}
	defer listener.Close()

	redirectURL := buildRedirectURL(listener.Addr(), c.callbackPath)
	authorizationURL, err := c.buildAuthorizationURL(redirectURL, state, codeChallenge)
	if err != nil {
		return LoginResult{}, err
	}

	session := LoginSession{
		State:            state,
		AuthorizationURL: authorizationURL,
		RedirectURL:      redirectURL,
		Scopes:           c.Scopes(),
	}

	callbackCh := make(chan callbackResult, 1)
	server := c.newCallbackServer(state, callbackCh)

	serveErrCh := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErrCh <- err
		}
	}()

	if err := browser.OpenURL(authorizationURL); err != nil {
		c.shutdownServer(server)
		return LoginResult{}, fmt.Errorf("ブラウザを開けませんでした。URL を手動で開いてください: %s: %w", authorizationURL, err)
	}

	timer := time.NewTimer(c.flowTimeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		c.shutdownServer(server)
		return LoginResult{}, fmt.Errorf("ログイン処理を中断しました: %w", ctx.Err())
	case err := <-serveErrCh:
		c.shutdownServer(server)
		return LoginResult{}, fmt.Errorf("localhost リダイレクト受信中に失敗しました: %w", err)
	case result := <-callbackCh:
		if result.err != nil {
			return LoginResult{}, result.err
		}
		return LoginResult{
			LoginSession: session,
			Code:         result.code,
			CodeVerifier: codeVerifier,
		}, nil
	case <-timer.C:
		c.shutdownServer(server)
		return LoginResult{}, fmt.Errorf("Google ログインが %s 以内に完了しませんでした", c.flowTimeout)
	}
}

func (c *Client) begin() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return errors.New("Google ログインはすでに進行中です")
	}

	c.running = true
	return nil
}

func (c *Client) finish() {
	c.mu.Lock()
	c.running = false
	c.mu.Unlock()
}

func (c *Client) buildAuthorizationURL(redirectURL string, state string, codeChallenge string) (string, error) {
	base, err := url.Parse(c.authorizationURL)
	if err != nil {
		return "", fmt.Errorf("Google OAuth エンドポイントの URL が不正です: %w", err)
	}

	query := base.Query()
	query.Set("client_id", c.clientID)
	query.Set("redirect_uri", redirectURL)
	query.Set("response_type", "code")
	query.Set("scope", strings.Join(c.scopes, " "))
	query.Set("state", state)
	query.Set("code_challenge", codeChallenge)
	query.Set("code_challenge_method", "S256")
	query.Set("access_type", "offline")
	query.Set("include_granted_scopes", "true")
	query.Set("prompt", "consent")
	base.RawQuery = query.Encode()

	return base.String(), nil
}

func (c *Client) newCallbackServer(expectedState string, callbackCh chan<- callbackResult) *http.Server {
	mux := http.NewServeMux()
	var server *http.Server
	mux.HandleFunc(c.callbackPath, func(w http.ResponseWriter, r *http.Request) {
		if errText := strings.TrimSpace(r.URL.Query().Get("error")); errText != "" {
			writeCallbackPage(w, http.StatusBadRequest, "Google ログインがキャンセルされました", "認証画面でエラーが返されました。アプリに戻って再試行してください。")
			callbackCh <- callbackResult{err: fmt.Errorf("Google ログインが拒否されました: %s", errText)}
			go c.shutdownServer(server)
			return
		}

		code := strings.TrimSpace(r.URL.Query().Get("code"))
		if code == "" {
			writeCallbackPage(w, http.StatusBadRequest, "認可コードが見つかりません", "Google から code パラメータを受け取れませんでした。")
			callbackCh <- callbackResult{err: errors.New("認可コードを受け取れませんでした")}
			go c.shutdownServer(server)
			return
		}

		receivedState := strings.TrimSpace(r.URL.Query().Get("state"))
		if receivedState != expectedState {
			writeCallbackPage(w, http.StatusBadRequest, "state 検証に失敗しました", "Google から戻った state が一致しないため、このログイン結果は破棄しました。")
			callbackCh <- callbackResult{err: errors.New("state 検証に失敗しました")}
			go c.shutdownServer(server)
			return
		}

		writeCallbackPage(w, http.StatusOK, "認可コードを受信しました", "このタブは閉じて、Mairu に戻ってください。次の処理でトークン交換へ進みます。")
		callbackCh <- callbackResult{code: code}
		go c.shutdownServer(server)
	})

	server = &http.Server{
		Handler: mux,
	}
	return server
}

func (c *Client) shutdownServer(server *http.Server) {
	if server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), callbackShutdownTimeout)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func buildRedirectURL(address net.Addr, callbackPath string) string {
	return fmt.Sprintf("http://%s%s", address.String(), callbackPath)
}

func randomToken(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func buildCodeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func writeCallbackPage(w http.ResponseWriter, status int, title string, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(
		w,
		`<!doctype html><html lang="ja"><head><meta charset="utf-8"><title>%s</title><style>body{margin:0;padding:32px;font-family:-apple-system,BlinkMacSystemFont,"Hiragino Sans","Noto Sans JP",sans-serif;background:#020617;color:#f8fafc}main{max-width:560px;margin:0 auto;padding:24px;border-radius:24px;border:1px solid rgba(148,163,184,.18);background:rgba(15,23,42,.88)}h1{margin:0 0 12px;font-size:1.4rem}p{margin:0;color:#cbd5e1;line-height:1.8}</style></head><body><main><h1>%s</h1><p>%s</p></main></body></html>`,
		html.EscapeString(title),
		html.EscapeString(title),
		html.EscapeString(message),
	)
}
