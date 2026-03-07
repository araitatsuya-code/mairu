package gmail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"mairu/internal/types"
)

const (
	labelsPath        = "/gmail/v1/users/me/labels"
	messageModifyPath = "/gmail/v1/users/me/messages/%s/modify"
	messageTrashPath  = "/gmail/v1/users/me/messages/%s/trash"

	systemLabelInbox  = "INBOX"
	systemLabelUnread = "UNREAD"

	mairuLabelImportant      = "Mairu/Important"
	mairuLabelNewsletter     = "Mairu/Newsletter"
	mairuLabelArchive        = "Mairu/Archive"
	mairuLabelUnreadPriority = "Mairu/Unread Priority"
	mairuLabelNeedsReview    = "Mairu/Needs Review"
)

type gmailLabel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type listLabelsResponse struct {
	Labels []gmailLabel `json:"labels"`
}

type createLabelRequest struct {
	Name                  string `json:"name"`
	LabelListVisibility   string `json:"labelListVisibility"`
	MessageListVisibility string `json:"messageListVisibility"`
}

type messageModifyRequest struct {
	AddLabelIDs    []string `json:"addLabelIds,omitempty"`
	RemoveLabelIDs []string `json:"removeLabelIds,omitempty"`
}

type actionPlan struct {
	messageID         string
	addLabelNames     []string
	addSystemLabelIDs []string
	removeLabelIDs    []string
	delete            bool
	hasArchive        bool
	hasMarkRead       bool
}

// ExecuteActions は承認済み分類結果を Gmail API の実アクションへ反映する。
func (c *Client) ExecuteActions(
	ctx context.Context,
	accessToken string,
	decisions []types.GmailActionDecision,
) (types.ExecuteGmailActionsResult, error) {
	trimmedToken := strings.TrimSpace(accessToken)
	if trimmedToken == "" {
		return types.ExecuteGmailActionsResult{}, fmt.Errorf("Gmail API 呼び出しに必要な access token がありません")
	}

	plans, requiredLabels, err := buildActionPlans(decisions)
	if err != nil {
		return types.ExecuteGmailActionsResult{}, err
	}

	labelIDs := make(map[string]string)
	createdLabels := make([]string, 0)
	if len(requiredLabels) > 0 {
		var ensureErr error
		labelIDs, createdLabels, ensureErr = c.ensureLabels(ctx, trimmedToken, requiredLabels)
		if ensureErr != nil {
			return types.ExecuteGmailActionsResult{}, ensureErr
		}
	}

	result := types.ExecuteGmailActionsResult{
		ProcessedCount: len(plans),
		CreatedLabels:  createdLabels,
	}

	for _, plan := range plans {
		if plan.delete {
			path := fmt.Sprintf(messageTrashPath, url.PathEscape(plan.messageID))
			if err := c.doJSONRequest(
				ctx,
				http.MethodPost,
				path,
				trimmedToken,
				"メールをゴミ箱へ移動",
				nil,
				nil,
			); err != nil {
				appendActionFailure(&result, plan.messageID, types.ActionKindDelete, err)
				continue
			}

			result.SuccessCount++
			result.DeletedCount++
			continue
		}

		payload := messageModifyRequest{
			AddLabelIDs:    append([]string(nil), plan.addSystemLabelIDs...),
			RemoveLabelIDs: append([]string(nil), plan.removeLabelIDs...),
		}
		for _, labelName := range plan.addLabelNames {
			labelID := strings.TrimSpace(labelIDs[labelName])
			if labelID == "" {
				return types.ExecuteGmailActionsResult{}, fmt.Errorf("ラベル %q の ID を解決できませんでした", labelName)
			}
			payload.AddLabelIDs = append(payload.AddLabelIDs, labelID)
		}

		path := fmt.Sprintf(messageModifyPath, url.PathEscape(plan.messageID))
		if err := c.doJSONRequest(
			ctx,
			http.MethodPost,
			path,
			trimmedToken,
			"メール更新",
			payload,
			nil,
		); err != nil {
			appendActionFailure(&result, plan.messageID, primaryActionKind(plan), err)
			continue
		}

		result.SuccessCount++
		if len(plan.addLabelNames) > 0 {
			result.LabeledCount++
		}
		if plan.hasArchive {
			result.ArchivedCount++
		}
		if plan.hasMarkRead {
			result.MarkedReadCount++
		}
	}

	result.FailureCount = len(result.Failures)
	result.Success = result.FailureCount == 0
	if result.Success {
		result.Message = fmt.Sprintf("Gmail アクションを %d 件実行しました。", result.SuccessCount)
	} else {
		result.Message = fmt.Sprintf(
			"Gmail アクションは %d 件成功、%d 件失敗しました。",
			result.SuccessCount,
			result.FailureCount,
		)
	}

	return result, nil
}

func buildActionPlans(
	decisions []types.GmailActionDecision,
) ([]actionPlan, []string, error) {
	if len(decisions) == 0 {
		return nil, nil, fmt.Errorf("実行対象のメールが選択されていません")
	}

	plans := make([]actionPlan, 0, len(decisions))
	requiredLabels := make(map[string]struct{})
	seenMessageIDs := make(map[string]struct{}, len(decisions))

	for index, decision := range decisions {
		messageID := strings.TrimSpace(decision.MessageID)
		if messageID == "" {
			return nil, nil, fmt.Errorf("decisions[%d].MessageID を入力してください", index)
		}
		if _, exists := seenMessageIDs[messageID]; exists {
			return nil, nil, fmt.Errorf("decisions[%d].MessageID %q が重複しています", index, messageID)
		}
		seenMessageIDs[messageID] = struct{}{}

		if !decision.Category.IsValid() {
			return nil, nil, fmt.Errorf("decisions[%d].Category %q は未対応です", index, decision.Category)
		}
		if !isReviewLevelSupported(decision.ReviewLevel) {
			return nil, nil, fmt.Errorf("decisions[%d].ReviewLevel %q は未対応です", index, decision.ReviewLevel)
		}

		plan := actionPlan{
			messageID:         messageID,
			addLabelNames:     make([]string, 0, 2),
			addSystemLabelIDs: make([]string, 0, 1),
			removeLabelIDs:    make([]string, 0, 2),
		}

		switch decision.Category {
		case types.ClassificationCategoryImportant:
			plan.addLabelNames = append(plan.addLabelNames, mairuLabelImportant)
		case types.ClassificationCategoryNewsletter:
			plan.addLabelNames = append(plan.addLabelNames, mairuLabelNewsletter)
			plan.hasMarkRead = true
			plan.removeLabelIDs = append(plan.removeLabelIDs, systemLabelUnread)
		case types.ClassificationCategoryJunk:
			plan.delete = true
		case types.ClassificationCategoryArchive:
			plan.addLabelNames = append(plan.addLabelNames, mairuLabelArchive)
			plan.hasArchive = true
			plan.removeLabelIDs = append(plan.removeLabelIDs, systemLabelInbox)
		case types.ClassificationCategoryUnreadPriority:
			plan.addLabelNames = append(plan.addLabelNames, mairuLabelUnreadPriority)
			plan.addSystemLabelIDs = append(plan.addSystemLabelIDs, systemLabelUnread)
		}

		if !plan.delete && (decision.ReviewLevel == types.ClassificationReviewLevelHold ||
			decision.ReviewLevel == types.ClassificationReviewLevelReviewWithReason) {
			plan.addLabelNames = append(plan.addLabelNames, mairuLabelNeedsReview)
		}

		plan.addLabelNames = uniqueStrings(plan.addLabelNames)
		plan.addSystemLabelIDs = uniqueStrings(plan.addSystemLabelIDs)
		plan.removeLabelIDs = uniqueStrings(plan.removeLabelIDs)
		for _, labelName := range plan.addLabelNames {
			requiredLabels[labelName] = struct{}{}
		}

		plans = append(plans, plan)
	}

	labelNames := make([]string, 0, len(requiredLabels))
	for labelName := range requiredLabels {
		labelNames = append(labelNames, labelName)
	}
	sort.Strings(labelNames)

	return plans, labelNames, nil
}

func isReviewLevelSupported(level types.ClassificationReviewLevel) bool {
	switch level {
	case types.ClassificationReviewLevelAutoApply,
		types.ClassificationReviewLevelReview,
		types.ClassificationReviewLevelReviewWithReason,
		types.ClassificationReviewLevelHold:
		return true
	default:
		return false
	}
}

func uniqueStrings(values []string) []string {
	if len(values) <= 1 {
		return values
	}

	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}

	return normalized
}

func primaryActionKind(plan actionPlan) types.ActionKind {
	switch {
	case plan.delete:
		return types.ActionKindDelete
	case plan.hasArchive:
		return types.ActionKindArchive
	case plan.hasMarkRead && len(plan.addLabelNames) == 0:
		return types.ActionKindMarkRead
	default:
		return types.ActionKindLabel
	}
}

func appendActionFailure(
	result *types.ExecuteGmailActionsResult,
	messageID string,
	kind types.ActionKind,
	err error,
) {
	result.Failures = append(result.Failures, types.GmailActionFailure{
		MessageID: messageID,
		Action:    kind,
		Error:     err.Error(),
	})
}

func (c *Client) ensureLabels(
	ctx context.Context,
	accessToken string,
	requiredLabels []string,
) (map[string]string, []string, error) {
	labelByName, err := c.listLabels(ctx, accessToken)
	if err != nil {
		return nil, nil, err
	}

	created := make([]string, 0)
	for _, labelName := range requiredLabels {
		if _, exists := labelByName[labelName]; exists {
			continue
		}

		label, err := c.createLabel(ctx, accessToken, labelName)
		if err != nil {
			refreshed, listErr := c.listLabels(ctx, accessToken)
			if listErr == nil {
				if labelID, exists := refreshed[labelName]; exists {
					labelByName[labelName] = labelID
					continue
				}
			}
			return nil, nil, err
		}
		labelByName[label.Name] = label.ID
		labelByName[labelName] = label.ID
		created = append(created, label.Name)
	}

	sort.Strings(created)
	return labelByName, created, nil
}

func (c *Client) listLabels(ctx context.Context, accessToken string) (map[string]string, error) {
	var response listLabelsResponse
	if err := c.doJSONRequest(
		ctx,
		http.MethodGet,
		labelsPath,
		accessToken,
		"ラベル一覧取得",
		nil,
		&response,
	); err != nil {
		return nil, err
	}

	labels := make(map[string]string, len(response.Labels))
	for _, label := range response.Labels {
		name := strings.TrimSpace(label.Name)
		id := strings.TrimSpace(label.ID)
		if name == "" || id == "" {
			continue
		}
		labels[name] = id
	}

	return labels, nil
}

func (c *Client) createLabel(
	ctx context.Context,
	accessToken string,
	labelName string,
) (gmailLabel, error) {
	name := strings.TrimSpace(labelName)
	if name == "" {
		return gmailLabel{}, fmt.Errorf("作成対象のラベル名が空です")
	}

	var response gmailLabel
	err := c.doJSONRequest(
		ctx,
		http.MethodPost,
		labelsPath,
		accessToken,
		"ラベル作成",
		createLabelRequest{
			Name:                  name,
			LabelListVisibility:   "labelShow",
			MessageListVisibility: "show",
		},
		&response,
	)
	if err != nil {
		return gmailLabel{}, err
	}

	response.ID = strings.TrimSpace(response.ID)
	response.Name = strings.TrimSpace(response.Name)
	if response.ID == "" || response.Name == "" {
		return gmailLabel{}, fmt.Errorf("ラベル作成 API の応答が不正です")
	}

	return response, nil
}

func (c *Client) doJSONRequest(
	ctx context.Context,
	method string,
	path string,
	accessToken string,
	operation string,
	requestBody any,
	responseBody any,
) error {
	fullURL := c.baseURL + path

	var bodyReader io.Reader
	if requestBody != nil {
		payload, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("Gmail API %s のリクエスト生成に失敗しました: %w", operation, err)
		}
		bodyReader = bytes.NewReader(payload)
	}

	request, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return fmt.Errorf("Gmail API %s のリクエスト作成に失敗しました: %w", operation, err)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("Gmail API %s に失敗しました: %w", operation, err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		var failure apiErrorResponse
		if decodeErr := json.NewDecoder(response.Body).Decode(&failure); decodeErr == nil {
			message := strings.TrimSpace(failure.Error.Message)
			if message != "" {
				return fmt.Errorf("Gmail API %s に失敗しました (%d): %s", operation, response.StatusCode, message)
			}
		}

		return fmt.Errorf("Gmail API %s に失敗しました (HTTP %d)", operation, response.StatusCode)
	}

	if responseBody == nil {
		return nil
	}

	if err := json.NewDecoder(response.Body).Decode(responseBody); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("Gmail API %s の応答読み取りに失敗しました: %w", operation, err)
	}

	return nil
}
