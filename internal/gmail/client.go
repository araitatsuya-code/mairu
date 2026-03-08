package gmail

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"mairu/internal/types"
)

const (
	defaultBaseURL     = "https://gmail.googleapis.com"
	profilePath        = "/gmail/v1/users/me/profile"
	messagesPath       = "/gmail/v1/users/me/messages"
	messageDetailPath  = "/gmail/v1/users/me/messages/%s"
	defaultFetchResult = types.ClassificationMaxBatchSize
	maxFetchResult     = 500
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
	Query      string
}

// FetchResult は取得したメール一覧を返す。
type FetchResult struct {
	Messages []types.EmailSummary
}

type listMessagesResponse struct {
	Messages []messageListItem `json:"messages"`
}

type messageListItem struct {
	ID       string `json:"id"`
	ThreadID string `json:"threadId"`
}

type messageDetailResponse struct {
	ID       string            `json:"id"`
	ThreadID string            `json:"threadId"`
	Snippet  string            `json:"snippet"`
	LabelIDs []string          `json:"labelIds"`
	Payload  messagePayloadDTO `json:"payload"`
}

type messagePayloadDTO struct {
	Headers []messageHeaderDTO `json:"headers"`
}

type messageHeaderDTO struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// FetchMessages は label/query 条件で Gmail メールを取得する。
func (c *Client) FetchMessages(
	ctx context.Context,
	accessToken string,
	request FetchRequest,
) (FetchResult, error) {
	trimmedToken := strings.TrimSpace(accessToken)
	if trimmedToken == "" {
		return FetchResult{}, fmt.Errorf("Gmail API 呼び出しに必要な access token がありません")
	}

	maxResults := request.MaxResults
	if maxResults <= 0 {
		maxResults = defaultFetchResult
	}
	if maxResults > maxFetchResult {
		maxResults = maxFetchResult
	}

	query := url.Values{}
	query.Set("maxResults", strconv.Itoa(maxResults))
	for _, labelID := range request.LabelIDs {
		trimmedLabelID := strings.TrimSpace(labelID)
		if trimmedLabelID == "" {
			continue
		}
		query.Add("labelIds", trimmedLabelID)
	}
	if value := strings.TrimSpace(request.Query); value != "" {
		query.Set("q", value)
	}

	path := messagesPath
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}

	var listed listMessagesResponse
	if err := c.doJSONRequest(
		ctx,
		http.MethodGet,
		path,
		trimmedToken,
		"メール一覧取得",
		nil,
		&listed,
	); err != nil {
		return FetchResult{}, err
	}

	items := listed.Messages
	if len(items) == 0 {
		return FetchResult{Messages: nil}, nil
	}

	messages := make([]types.EmailSummary, 0, len(items))
	for _, item := range items {
		messageID := strings.TrimSpace(item.ID)
		if messageID == "" {
			continue
		}

		var detail messageDetailResponse
		detailPath := fmt.Sprintf(
			"%s?format=metadata&metadataHeaders=From&metadataHeaders=Subject",
			fmt.Sprintf(messageDetailPath, url.PathEscape(messageID)),
		)
		if err := c.doJSONRequest(
			ctx,
			http.MethodGet,
			detailPath,
			trimmedToken,
			"メール詳細取得",
			nil,
			&detail,
		); err != nil {
			return FetchResult{}, err
		}

		threadID := strings.TrimSpace(detail.ThreadID)
		if threadID == "" {
			threadID = strings.TrimSpace(item.ThreadID)
		}
		messages = append(messages, types.EmailSummary{
			ID:       messageID,
			ThreadID: threadID,
			From:     messageHeaderValue(detail.Payload.Headers, "From"),
			Subject:  messageHeaderValue(detail.Payload.Headers, "Subject"),
			Snippet:  strings.TrimSpace(detail.Snippet),
			Unread:   messageHasLabel(detail.LabelIDs, systemLabelUnread),
		})
	}

	return FetchResult{Messages: messages}, nil
}

func messageHeaderValue(headers []messageHeaderDTO, headerName string) string {
	target := strings.ToLower(strings.TrimSpace(headerName))
	if target == "" {
		return ""
	}

	for _, header := range headers {
		name := strings.ToLower(strings.TrimSpace(header.Name))
		if name != target {
			continue
		}
		return strings.TrimSpace(header.Value)
	}
	return ""
}

func messageHasLabel(labels []string, labelID string) bool {
	target := strings.TrimSpace(labelID)
	if target == "" {
		return false
	}
	for _, label := range labels {
		if strings.TrimSpace(label) == target {
			return true
		}
	}
	return false
}
