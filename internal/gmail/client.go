package gmail

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"mairu/internal/types"
)

const (
	defaultBaseURL = "https://gmail.googleapis.com"
	profilePath    = "/gmail/v1/users/me/profile"
)

// Options は Gmail API クライアント生成時の設定をまとめる。
type Options struct {
	BaseURL    string
	HTTPClient *http.Client
}

// Client は Gmail API クライアントの最小実装。
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// Profile は接続確認で利用する Gmail アカウント情報を表す。
type Profile struct {
	EmailAddress  string
	MessagesTotal int64
	ThreadsTotal  int64
	HistoryID     string
}

type profileResponse struct {
	EmailAddress  string `json:"emailAddress"`
	MessagesTotal int64  `json:"messagesTotal"`
	ThreadsTotal  int64  `json:"threadsTotal"`
	HistoryID     string `json:"historyId"`
}

type apiErrorResponse struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// NewClient は Gmail API クライアントを初期化する。
func NewClient(options Options) *Client {
	baseURL := strings.TrimSpace(options.BaseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

// CheckConnection は Gmail プロフィール取得で接続確認を行う。
func (c *Client) CheckConnection(ctx context.Context, accessToken string) (Profile, error) {
	trimmedToken := strings.TrimSpace(accessToken)
	if trimmedToken == "" {
		return Profile{}, fmt.Errorf("Gmail API 呼び出しに必要な access token がありません")
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		c.baseURL+profilePath,
		nil,
	)
	if err != nil {
		return Profile{}, fmt.Errorf("Gmail API リクエストの作成に失敗しました: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+trimmedToken)

	response, err := c.httpClient.Do(request)
	if err != nil {
		return Profile{}, fmt.Errorf("Gmail API へ接続できませんでした: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		var failure apiErrorResponse
		if decodeErr := json.NewDecoder(response.Body).Decode(&failure); decodeErr == nil {
			message := strings.TrimSpace(failure.Error.Message)
			if message != "" {
				return Profile{}, fmt.Errorf("Gmail API 接続確認に失敗しました (%d): %s", response.StatusCode, message)
			}
		}

		return Profile{}, fmt.Errorf("Gmail API 接続確認に失敗しました (HTTP %d)", response.StatusCode)
	}

	var payload profileResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return Profile{}, fmt.Errorf("Gmail API 応答の読み取りに失敗しました: %w", err)
	}
	if strings.TrimSpace(payload.EmailAddress) == "" {
		return Profile{}, fmt.Errorf("Gmail API 応答に emailAddress が含まれていません")
	}

	return Profile{
		EmailAddress:  payload.EmailAddress,
		MessagesTotal: payload.MessagesTotal,
		ThreadsTotal:  payload.ThreadsTotal,
		HistoryID:     payload.HistoryID,
	}, nil
}

// FetchRequest はメール取得条件の最小セットを表す。
type FetchRequest struct {
	MaxResults int
	LabelIDs   []string
}

// FetchResult は取得したメール一覧を返す。
type FetchResult struct {
	Messages []types.EmailSummary
}
