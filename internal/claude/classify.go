package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"mairu/internal/types"
)

const classificationSystemPrompt = "あなたは Gmail 整理アシスタントです。必ず JSON だけを返し、説明文や Markdown を混ぜないでください。"

type apiMessageRequest struct {
	Model       string       `json:"model"`
	MaxTokens   int          `json:"max_tokens"`
	Temperature float64      `json:"temperature"`
	System      string       `json:"system"`
	Messages    []apiMessage `json:"messages"`
}

type apiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type apiResponse struct {
	Content []apiContentBlock `json:"content"`
	Error   apiErrorPayload   `json:"error"`
}

type apiContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type apiErrorPayload struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type promptMessage struct {
	ID      string `json:"id"`
	From    string `json:"from"`
	Subject string `json:"subject"`
	Snippet string `json:"snippet"`
	Unread  bool   `json:"unread"`
}

type classificationResultPayload struct {
	ID         string  `json:"id"`
	Category   string  `json:"category"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

type classificationEnvelope struct {
	Results []classificationResultPayload `json:"results"`
}

// Classify は Claude Messages API を使ってメール一覧を分類する。
func (c *Client) Classify(ctx context.Context, apiKey string, request types.ClassificationRequest) (types.ClassificationResponse, error) {
	trimmedAPIKey := strings.TrimSpace(apiKey)
	if trimmedAPIKey == "" {
		return types.ClassificationResponse{}, fmt.Errorf("Claude API 呼び出しに必要な API キーがありません")
	}

	messages, err := validateMessages(request.Messages, c.maxBatchSize)
	if err != nil {
		return types.ClassificationResponse{}, err
	}

	model := strings.TrimSpace(request.Model)
	if model == "" {
		model = c.defaultModel
	}

	body := apiMessageRequest{
		Model:       model,
		MaxTokens:   c.maxTokens,
		Temperature: 0,
		System:      classificationSystemPrompt,
		Messages: []apiMessage{
			{
				Role:    "user",
				Content: buildClassificationPrompt(messages),
			},
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return types.ClassificationResponse{}, fmt.Errorf("Claude API リクエストの組み立てに失敗しました: %w", err)
	}

	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+messagesPath,
		bytes.NewReader(payload),
	)
	if err != nil {
		return types.ClassificationResponse{}, fmt.Errorf("Claude API リクエストの作成に失敗しました: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("x-api-key", trimmedAPIKey)
	httpRequest.Header.Set("anthropic-version", c.apiVersion)

	response, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return types.ClassificationResponse{}, fmt.Errorf("Claude API へ接続できませんでした: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return types.ClassificationResponse{}, decodeAPIError(response)
	}

	text, err := extractResponseText(response.Body)
	if err != nil {
		return types.ClassificationResponse{}, err
	}

	results, err := parseClassificationResults(text, messages)
	if err != nil {
		return types.ClassificationResponse{}, err
	}

	return types.ClassificationResponse{
		Model:   model,
		Results: results,
	}, nil
}

func validateMessages(messages []types.EmailSummary, maxBatchSize int) ([]types.EmailSummary, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("Claude 分類対象のメールがありません")
	}
	if len(messages) > maxBatchSize {
		return nil, fmt.Errorf("Claude 分類は最大 %d 件までです", maxBatchSize)
	}

	normalized := make([]types.EmailSummary, 0, len(messages))
	seenIDs := make(map[string]struct{}, len(messages))
	for index, message := range messages {
		id := strings.TrimSpace(message.ID)
		if id == "" {
			return nil, fmt.Errorf("messages[%d].ID を入力してください", index)
		}
		if _, exists := seenIDs[id]; exists {
			return nil, fmt.Errorf("messages[%d].ID %q が重複しています", index, id)
		}
		seenIDs[id] = struct{}{}

		normalized = append(normalized, types.EmailSummary{
			ID:       id,
			ThreadID: strings.TrimSpace(message.ThreadID),
			From:     strings.TrimSpace(message.From),
			Subject:  strings.TrimSpace(message.Subject),
			Snippet:  strings.TrimSpace(message.Snippet),
			Unread:   message.Unread,
		})
	}

	return normalized, nil
}

func buildClassificationPrompt(messages []types.EmailSummary) string {
	payload := make([]promptMessage, 0, len(messages))
	for _, message := range messages {
		payload = append(payload, promptMessage{
			ID:      message.ID,
			From:    message.From,
			Subject: message.Subject,
			Snippet: message.Snippet,
			Unread:  message.Unread,
		})
	}

	serialized, err := json.Marshal(payload)
	if err != nil {
		return "[]"
	}

	return strings.Join([]string{
		"次のメールを 1 通ずつ分類してください。",
		"使用できる category は important, newsletter, junk, archive, unread_priority のみです。",
		"confidence は 0 以上 1 以下の小数、reason は日本語の短い説明にしてください。",
		"必ず JSON 配列のみを返し、各要素に id, category, confidence, reason を含めてください。",
		"メール情報: " + string(serialized),
	}, "\n")
}

func decodeAPIError(response *http.Response) error {
	var failure apiResponse
	if err := json.NewDecoder(response.Body).Decode(&failure); err == nil {
		message := strings.TrimSpace(failure.Error.Message)
		if message != "" {
			return fmt.Errorf("Claude API 分類に失敗しました (%d): %s", response.StatusCode, message)
		}
	}

	return fmt.Errorf("Claude API 分類に失敗しました (HTTP %d)", response.StatusCode)
}

func extractResponseText(body io.Reader) (string, error) {
	var payload apiResponse
	if err := json.NewDecoder(body).Decode(&payload); err != nil {
		return "", fmt.Errorf("Claude API 応答の読み取りに失敗しました: %w", err)
	}

	var builder strings.Builder
	for _, block := range payload.Content {
		if block.Type != "text" {
			continue
		}
		text := strings.TrimSpace(block.Text)
		if text == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(text)
	}

	if builder.Len() == 0 {
		return "", fmt.Errorf("Claude API 応答に分類結果テキストが含まれていません")
	}

	return trimCodeFence(builder.String()), nil
}

func trimCodeFence(value string) string {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "```") {
		return trimmed
	}

	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)
	if strings.HasPrefix(trimmed, "json") {
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "json"))
	}

	if index := strings.LastIndex(trimmed, "```"); index >= 0 {
		trimmed = strings.TrimSpace(trimmed[:index])
	}

	return trimmed
}

func parseClassificationResults(raw string, messages []types.EmailSummary) ([]types.ClassificationResult, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("Claude API 応答が空です")
	}

	var direct []classificationResultPayload
	if err := json.Unmarshal([]byte(trimmed), &direct); err == nil {
		return normalizeResults(direct, messages)
	}

	var wrapped classificationEnvelope
	if err := json.Unmarshal([]byte(trimmed), &wrapped); err == nil {
		return normalizeResults(wrapped.Results, messages)
	}

	return nil, fmt.Errorf("Claude API 応答を分類結果 JSON として解釈できません")
}

func normalizeResults(entries []classificationResultPayload, messages []types.EmailSummary) ([]types.ClassificationResult, error) {
	if len(entries) != len(messages) {
		return nil, fmt.Errorf("Claude API 応答件数が一致しません: got %d, want %d", len(entries), len(messages))
	}

	order := make([]string, 0, len(messages))
	expected := make(map[string]struct{}, len(messages))
	for _, message := range messages {
		order = append(order, message.ID)
		expected[message.ID] = struct{}{}
	}

	normalized := make(map[string]types.ClassificationResult, len(entries))
	for index, entry := range entries {
		id := strings.TrimSpace(entry.ID)
		if id == "" {
			return nil, fmt.Errorf("Claude API 応答 results[%d].id が空です", index)
		}
		if _, ok := expected[id]; !ok {
			return nil, fmt.Errorf("Claude API 応答に想定外の id %q が含まれています", id)
		}
		if _, exists := normalized[id]; exists {
			return nil, fmt.Errorf("Claude API 応答に id %q が重複しています", id)
		}

		category := types.ClassificationCategory(strings.TrimSpace(entry.Category))
		if !category.IsValid() {
			return nil, fmt.Errorf("Claude API 応答 results[%d].category %q は未対応です", index, entry.Category)
		}
		if entry.Confidence < 0 || entry.Confidence > 1 {
			return nil, fmt.Errorf("Claude API 応答 results[%d].confidence は 0〜1 の範囲で指定してください", index)
		}

		reason := strings.TrimSpace(entry.Reason)
		if reason == "" {
			return nil, fmt.Errorf("Claude API 応答 results[%d].reason が空です", index)
		}

		normalized[id] = types.ClassificationResult{
			MessageID:   id,
			Category:    category,
			Confidence:  entry.Confidence,
			Reason:      reason,
			ReviewLevel: types.ReviewLevelForConfidence(entry.Confidence),
		}
	}

	results := make([]types.ClassificationResult, 0, len(order))
	for _, id := range order {
		result, ok := normalized[id]
		if !ok {
			return nil, fmt.Errorf("Claude API 応答に id %q の結果がありません", id)
		}
		results = append(results, result)
	}

	return results, nil
}
