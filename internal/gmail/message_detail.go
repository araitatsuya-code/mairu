package gmail

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"mairu/internal/types"
)

type messageBodyDTO struct {
	Data string `json:"data"`
}

type messageDetailPayloadDTO struct {
	MimeType string                    `json:"mimeType"`
	Headers  []messageHeaderDTO        `json:"headers"`
	Body     messageBodyDTO            `json:"body"`
	Parts    []messageDetailPayloadDTO `json:"parts"`
}

type fullMessageDetailResponse struct {
	ID       string                  `json:"id"`
	ThreadID string                  `json:"threadId"`
	Snippet  string                  `json:"snippet"`
	LabelIDs []string                `json:"labelIds"`
	Payload  messageDetailPayloadDTO `json:"payload"`
}

// FetchMessageDetail は Gmail メール 1 通分の詳細情報を取得する。
func (c *Client) FetchMessageDetail(
	ctx context.Context,
	accessToken string,
	messageID string,
) (types.GmailMessageDetail, error) {
	trimmedToken := strings.TrimSpace(accessToken)
	if trimmedToken == "" {
		return types.GmailMessageDetail{}, fmt.Errorf("Gmail API 呼び出しに必要な access token がありません")
	}
	trimmedMessageID := strings.TrimSpace(messageID)
	if trimmedMessageID == "" {
		return types.GmailMessageDetail{}, fmt.Errorf("messageID を入力してください")
	}

	var detail fullMessageDetailResponse
	detailPath := fmt.Sprintf("%s?format=full", fmt.Sprintf(messageDetailPath, url.PathEscape(trimmedMessageID)))
	if err := c.doJSONRequest(
		ctx,
		http.MethodGet,
		detailPath,
		trimmedToken,
		"メール詳細取得",
		nil,
		&detail,
	); err != nil {
		return types.GmailMessageDetail{}, err
	}

	bodyText, bodyHTML := collectMessageBodies(detail.Payload)
	headers := sanitizeHeaders(detail.Payload.Headers)

	result := types.GmailMessageDetail{
		ID:       strings.TrimSpace(detail.ID),
		ThreadID: strings.TrimSpace(detail.ThreadID),
		From:     messageHeaderValue(detail.Payload.Headers, "From"),
		To:       messageHeaderValue(detail.Payload.Headers, "To"),
		Subject:  messageHeaderValue(detail.Payload.Headers, "Subject"),
		Date:     messageHeaderValue(detail.Payload.Headers, "Date"),
		Snippet:  strings.TrimSpace(detail.Snippet),
		LabelIDs: sanitizeLabelIDs(detail.LabelIDs),
		Unread:   messageHasLabel(detail.LabelIDs, systemLabelUnread),
		BodyText: bodyText,
		BodyHTML: bodyHTML,
		Headers:  headers,
	}

	if result.ID == "" {
		result.ID = trimmedMessageID
	}

	return result, nil
}

func collectMessageBodies(payload messageDetailPayloadDTO) (string, string) {
	textBodies := make([]string, 0, 4)
	htmlBodies := make([]string, 0, 2)
	collectMessageBodyPart(payload, &textBodies, &htmlBodies)

	return strings.TrimSpace(strings.Join(textBodies, "\n\n")), strings.TrimSpace(strings.Join(htmlBodies, "\n\n"))
}

func collectMessageBodyPart(
	payload messageDetailPayloadDTO,
	textBodies *[]string,
	htmlBodies *[]string,
) {
	mimeType := strings.ToLower(strings.TrimSpace(payload.MimeType))
	decoded := decodeMessageBodyData(payload.Body.Data)

	switch mimeType {
	case "text/plain":
		if decoded != "" {
			*textBodies = append(*textBodies, decoded)
		}
	case "text/html":
		if decoded != "" {
			*htmlBodies = append(*htmlBodies, decoded)
		}
	default:
		if len(payload.Parts) == 0 && decoded != "" {
			if strings.Contains(mimeType, "html") {
				*htmlBodies = append(*htmlBodies, decoded)
			} else {
				*textBodies = append(*textBodies, decoded)
			}
		}
	}

	for _, child := range payload.Parts {
		collectMessageBodyPart(child, textBodies, htmlBodies)
	}
}

func decodeMessageBodyData(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	decoded, err := base64.RawURLEncoding.DecodeString(trimmed)
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(trimmed)
		if err != nil {
			return ""
		}
	}

	return strings.TrimSpace(string(decoded))
}

func sanitizeHeaders(headers []messageHeaderDTO) []types.GmailMessageHeader {
	sanitized := make([]types.GmailMessageHeader, 0, len(headers))
	for _, header := range headers {
		name := strings.TrimSpace(header.Name)
		value := strings.TrimSpace(header.Value)
		if name == "" || value == "" {
			continue
		}
		sanitized = append(sanitized, types.GmailMessageHeader{
			Name:  name,
			Value: value,
		})
	}
	return sanitized
}

func sanitizeLabelIDs(labelIDs []string) []string {
	sanitized := make([]string, 0, len(labelIDs))
	for _, labelID := range labelIDs {
		trimmed := strings.TrimSpace(labelID)
		if trimmed == "" {
			continue
		}
		sanitized = append(sanitized, trimmed)
	}
	return sanitized
}
