package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultTokenURL  = "https://oauth2.googleapis.com/token"
	tokenRefreshSkew = time.Minute
)

// TokenSet は保存対象の OAuth トークン一式を表す。
type TokenSet struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	Scope        string    `json:"scope,omitempty"`
	IDToken      string    `json:"id_token,omitempty"`
	Expiry       time.Time `json:"expiry"`
}

// Scopes はトークン応答に含まれたスコープを分割して返す。
func (t TokenSet) Scopes() []string {
	if strings.TrimSpace(t.Scope) == "" {
		return nil
	}
	return strings.Fields(t.Scope)
}

type tokenExchangeResponse struct {
	AccessToken      string `json:"access_token"`
	ExpiresInSeconds int    `json:"expires_in"`
	RefreshToken     string `json:"refresh_token"`
	Scope            string `json:"scope"`
	TokenType        string `json:"token_type"`
	IDToken          string `json:"id_token"`
}

type tokenExchangeError struct {
	Code        string `json:"error"`
	Description string `json:"error_description"`
}

// ExchangeCode は認可コードをアクセストークンへ交換する。
func (c *Client) ExchangeCode(ctx context.Context, loginResult LoginResult) (TokenSet, error) {
	tokenURL := strings.TrimSpace(c.tokenURL)
	if tokenURL == "" {
		tokenURL = defaultTokenURL
	}

	form := url.Values{}
	form.Set("client_id", c.clientID)
	if strings.TrimSpace(c.clientSecret) != "" {
		form.Set("client_secret", c.clientSecret)
	}
	form.Set("code", loginResult.Code)
	form.Set("code_verifier", loginResult.CodeVerifier)
	form.Set("grant_type", "authorization_code")
	form.Set("redirect_uri", loginResult.RedirectURL)

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		tokenURL,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return TokenSet{}, fmt.Errorf("トークン交換リクエストの作成に失敗しました: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	response, err := httpClient.Do(request)
	if err != nil {
		return TokenSet{}, fmt.Errorf("Google トークンエンドポイントへ接続できませんでした: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		var failure tokenExchangeError
		if decodeErr := json.NewDecoder(response.Body).Decode(&failure); decodeErr == nil {
			message := strings.TrimSpace(failure.Description)
			if message == "" {
				message = strings.TrimSpace(failure.Code)
			}
			if message != "" {
				return TokenSet{}, fmt.Errorf("Google トークン交換に失敗しました (%d): %s", response.StatusCode, message)
			}
		}

		return TokenSet{}, fmt.Errorf("Google トークン交換に失敗しました (HTTP %d)", response.StatusCode)
	}

	var payload tokenExchangeResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return TokenSet{}, fmt.Errorf("Google トークン応答の読み取りに失敗しました: %w", err)
	}

	if strings.TrimSpace(payload.AccessToken) == "" {
		return TokenSet{}, fmt.Errorf("Google トークン応答に access_token が含まれていません")
	}

	expiry := time.Now().UTC()
	if payload.ExpiresInSeconds > 0 {
		expiry = expiry.Add(time.Duration(payload.ExpiresInSeconds) * time.Second)
	}

	return TokenSet{
		AccessToken:  payload.AccessToken,
		RefreshToken: payload.RefreshToken,
		TokenType:    payload.TokenType,
		Scope:        payload.Scope,
		IDToken:      payload.IDToken,
		Expiry:       expiry,
	}, nil
}

// EnsureValidToken は保存済みトークンを再利用可能な状態に整える。
func (c *Client) EnsureValidToken(ctx context.Context, token TokenSet) (TokenSet, bool, error) {
	if strings.TrimSpace(token.AccessToken) == "" {
		return TokenSet{}, false, errors.New("保存済み OAuth トークンに access_token がありません")
	}

	if !shouldRefreshToken(token.Expiry) {
		return token, false, nil
	}

	refreshed, err := c.RefreshToken(ctx, token)
	if err != nil {
		return TokenSet{}, false, err
	}

	return refreshed, true, nil
}

// RefreshToken は保存済み refresh token を使って新しい access token を取得する。
func (c *Client) RefreshToken(ctx context.Context, token TokenSet) (TokenSet, error) {
	refreshToken := strings.TrimSpace(token.RefreshToken)
	if refreshToken == "" {
		return TokenSet{}, errors.New("保存済み OAuth トークンに refresh_token がありません")
	}

	tokenURL := strings.TrimSpace(c.tokenURL)
	if tokenURL == "" {
		tokenURL = defaultTokenURL
	}

	form := url.Values{}
	form.Set("client_id", c.clientID)
	if strings.TrimSpace(c.clientSecret) != "" {
		form.Set("client_secret", c.clientSecret)
	}
	form.Set("refresh_token", refreshToken)
	form.Set("grant_type", "refresh_token")

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		tokenURL,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return TokenSet{}, fmt.Errorf("トークン更新リクエストの作成に失敗しました: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	response, err := httpClient.Do(request)
	if err != nil {
		return TokenSet{}, fmt.Errorf("Google トークン更新エンドポイントへ接続できませんでした: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		var failure tokenExchangeError
		if decodeErr := json.NewDecoder(response.Body).Decode(&failure); decodeErr == nil {
			message := strings.TrimSpace(failure.Description)
			if message == "" {
				message = strings.TrimSpace(failure.Code)
			}
			if message != "" {
				return TokenSet{}, fmt.Errorf("Google トークン更新に失敗しました (%d): %s", response.StatusCode, message)
			}
		}

		return TokenSet{}, fmt.Errorf("Google トークン更新に失敗しました (HTTP %d)", response.StatusCode)
	}

	var payload tokenExchangeResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return TokenSet{}, fmt.Errorf("Google トークン更新応答の読み取りに失敗しました: %w", err)
	}

	if strings.TrimSpace(payload.AccessToken) == "" {
		return TokenSet{}, fmt.Errorf("Google トークン更新応答に access_token が含まれていません")
	}

	expiry := time.Now().UTC()
	if payload.ExpiresInSeconds > 0 {
		expiry = expiry.Add(time.Duration(payload.ExpiresInSeconds) * time.Second)
	}

	return TokenSet{
		AccessToken:  payload.AccessToken,
		RefreshToken: firstNonEmpty(payload.RefreshToken, token.RefreshToken),
		TokenType:    firstNonEmpty(payload.TokenType, token.TokenType),
		Scope:        firstNonEmpty(payload.Scope, token.Scope),
		IDToken:      firstNonEmpty(payload.IDToken, token.IDToken),
		Expiry:       expiry,
	}, nil
}

func shouldRefreshToken(expiry time.Time) bool {
	if expiry.IsZero() {
		return false
	}

	return !time.Now().UTC().Add(tokenRefreshSkew).Before(expiry)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return value
		}
	}

	return ""
}
