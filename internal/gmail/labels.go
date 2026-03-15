package gmail

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"mairu/internal/types"
)

// ListLabels は Gmail ラベル一覧を取得する。
func (c *Client) ListLabels(ctx context.Context, accessToken string) ([]types.GmailLabel, error) {
	trimmedToken := strings.TrimSpace(accessToken)
	if trimmedToken == "" {
		return nil, fmt.Errorf("Gmail API 呼び出しに必要な access token がありません")
	}

	var response listLabelsResponse
	if err := c.doJSONRequest(
		ctx,
		http.MethodGet,
		labelsPath,
		trimmedToken,
		"ラベル一覧取得",
		nil,
		&response,
	); err != nil {
		return nil, err
	}

	labels := make([]types.GmailLabel, 0, len(response.Labels))
	for _, label := range response.Labels {
		id := strings.TrimSpace(label.ID)
		name := strings.TrimSpace(label.Name)
		labelType := strings.TrimSpace(label.Type)
		if id == "" || name == "" {
			continue
		}
		labels = append(labels, types.GmailLabel{
			ID:   id,
			Name: name,
			Type: labelType,
		})
	}

	sort.Slice(labels, func(i int, j int) bool {
		left := strings.ToLower(labels[i].Name)
		right := strings.ToLower(labels[j].Name)
		if left == right {
			return labels[i].ID < labels[j].ID
		}
		return left < right
	})

	return labels, nil
}
