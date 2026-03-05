package claude

import (
	"net/http"
	"strings"

	"mairu/internal/types"
)

const (
	defaultBaseURL    = "https://api.anthropic.com"
	defaultAPIVersion = "2023-06-01"
	defaultModel      = "claude-sonnet-4-5-20250929"
	defaultMaxTokens  = 2048
	messagesPath      = "/v1/messages"
)

// Options は Claude API クライアント生成時の設定をまとめる。
type Options struct {
	BaseURL      string
	APIVersion   string
	DefaultModel string
	MaxTokens    int
	MaxBatchSize int
	HTTPClient   *http.Client
}

// Client は Claude Messages API を呼び出す最小実装。
type Client struct {
	baseURL      string
	apiVersion   string
	defaultModel string
	maxTokens    int
	maxBatchSize int
	httpClient   *http.Client
}

// NewClient は Claude API クライアントを初期化する。
func NewClient(options Options) *Client {
	baseURL := strings.TrimSpace(options.BaseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	apiVersion := strings.TrimSpace(options.APIVersion)
	if apiVersion == "" {
		apiVersion = defaultAPIVersion
	}

	defaultModelName := strings.TrimSpace(options.DefaultModel)
	if defaultModelName == "" {
		defaultModelName = defaultModel
	}

	maxTokens := options.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	maxBatchSize := options.MaxBatchSize
	if maxBatchSize <= 0 {
		maxBatchSize = types.ClassificationMaxBatchSize
	}

	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		apiVersion:   apiVersion,
		defaultModel: defaultModelName,
		maxTokens:    maxTokens,
		maxBatchSize: maxBatchSize,
		httpClient:   httpClient,
	}
}
