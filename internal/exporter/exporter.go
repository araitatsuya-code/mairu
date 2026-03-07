package exporter

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"mairu/internal/types"
)

type exportEnvelope[T any] struct {
	ExportedAt string `json:"exportedAt"`
	Items      []T    `json:"items"`
}

type blocklistJSONEntry struct {
	Kind      types.BlocklistKind `json:"kind"`
	Pattern   string              `json:"pattern"`
	Note      string              `json:"note"`
	CreatedAt string              `json:"createdAt,omitempty"`
	UpdatedAt string              `json:"updatedAt,omitempty"`
}

type dailyLogSummary struct {
	Date             string `json:"date"`
	Total            int    `json:"total"`
	Important        int    `json:"important"`
	Newsletter       int    `json:"newsletter"`
	Junk             int    `json:"junk"`
	Archive          int    `json:"archive"`
	UnreadPriority   int    `json:"unreadPriority"`
	AutoApply        int    `json:"autoApply"`
	Review           int    `json:"review"`
	ReviewWithReason int    `json:"reviewWithReason"`
	Hold             int    `json:"hold"`
	Claude           int    `json:"claude"`
	Blocklist        int    `json:"blocklist"`
}

func MarshalProcessedMailCSV(entries []types.ActionLogEntry) ([]byte, error) {
	buffer := &bytes.Buffer{}
	writer := csv.NewWriter(buffer)
	if err := writer.Write([]string{
		"executed_at",
		"message_id",
		"thread_id",
		"from",
		"subject",
		"category",
		"confidence",
		"review_level",
		"source",
		"action_kind",
		"status",
		"detail",
	}); err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if err := writer.Write([]string{
			entry.CreatedAt,
			entry.MessageID,
			entry.ThreadID,
			entry.From,
			entry.Subject,
			string(entry.Category),
			formatConfidence(entry.Confidence),
			string(entry.ReviewLevel),
			string(entry.Source),
			string(entry.ActionKind),
			entry.Status,
			entry.Detail,
		}); err != nil {
			return nil, err
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

func MarshalProcessedMailJSON(entries []types.ActionLogEntry, exportedAt time.Time) ([]byte, error) {
	return marshalIndented(exportEnvelope[types.ActionLogEntry]{
		ExportedAt: exportedAt.Format(time.RFC3339),
		Items:      entries,
	})
}

func MarshalBlocklistJSON(entries []types.BlocklistEntry, exportedAt time.Time) ([]byte, error) {
	items := make([]blocklistJSONEntry, 0, len(entries))
	for _, entry := range entries {
		items = append(items, blocklistJSONEntry{
			Kind:      entry.Kind,
			Pattern:   entry.Pattern,
			Note:      entry.Note,
			CreatedAt: entry.CreatedAt,
			UpdatedAt: entry.UpdatedAt,
		})
	}

	return marshalIndented(exportEnvelope[blocklistJSONEntry]{
		ExportedAt: exportedAt.Format(time.RFC3339),
		Items:      items,
	})
}

func ParseBlocklistJSON(data []byte) ([]types.UpsertBlocklistEntryRequest, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("blocklist JSON が空です")
	}

	var wrapped exportEnvelope[blocklistJSONEntry]
	if err := json.Unmarshal(trimmed, &wrapped); err == nil && len(wrapped.Items) > 0 {
		return normalizeImportedBlocklist(wrapped.Items)
	}

	var direct []blocklistJSONEntry
	if err := json.Unmarshal(trimmed, &direct); err == nil {
		return normalizeImportedBlocklist(direct)
	}

	return nil, fmt.Errorf("blocklist JSON の形式が不正です")
}

func MarshalImportantSummaryCSV(entries []types.ClassificationLogEntry) ([]byte, error) {
	buffer := &bytes.Buffer{}
	writer := csv.NewWriter(buffer)
	if err := writer.Write([]string{
		"classified_at",
		"message_id",
		"thread_id",
		"from",
		"subject",
		"snippet",
		"confidence",
		"review_level",
		"source",
	}); err != nil {
		return nil, err
	}

	for _, entry := range filterImportant(entries) {
		if err := writer.Write([]string{
			entry.ClassifiedAt,
			entry.MessageID,
			entry.ThreadID,
			entry.From,
			entry.Subject,
			entry.Snippet,
			formatConfidence(entry.Confidence),
			string(entry.ReviewLevel),
			string(entry.Source),
		}); err != nil {
			return nil, err
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

func MarshalImportantSummaryPDF(entries []types.ClassificationLogEntry, exportedAt time.Time) ([]byte, error) {
	important := filterImportant(entries)
	lines := []string{
		"Mairu 重要メールサマリー",
		fmt.Sprintf("出力日時: %s", exportedAt.Format("2006-01-02 15:04:05")),
		"",
	}

	if len(important) == 0 {
		lines = append(lines, "重要メールはまだ記録されていません。")
		return BuildSimplePDF(lines)
	}

	for index, entry := range important {
		lines = append(lines,
			fmt.Sprintf("%d. %s", index+1, compactText(entry.Subject, 32)),
			fmt.Sprintf("   差出人: %s", compactText(entry.From, 32)),
			fmt.Sprintf("   信頼度: %s / 判定: %s / 日時: %s",
				formatConfidence(entry.Confidence), entry.ReviewLevel, entry.ClassifiedAt),
			fmt.Sprintf("   概要: %s", compactText(entry.Snippet, 42)),
			"",
		)
	}

	return BuildSimplePDF(lines)
}

func MarshalDailyLogsCSV(entries []types.ClassificationLogEntry) ([]byte, error) {
	summaries := buildDailyLogSummaries(entries)

	buffer := &bytes.Buffer{}
	writer := csv.NewWriter(buffer)
	if err := writer.Write([]string{
		"date",
		"total",
		"important",
		"newsletter",
		"junk",
		"archive",
		"unread_priority",
		"auto_apply",
		"review",
		"review_with_reason",
		"hold",
		"claude",
		"blocklist",
	}); err != nil {
		return nil, err
	}

	for _, item := range summaries {
		if err := writer.Write([]string{
			item.Date,
			strconv.Itoa(item.Total),
			strconv.Itoa(item.Important),
			strconv.Itoa(item.Newsletter),
			strconv.Itoa(item.Junk),
			strconv.Itoa(item.Archive),
			strconv.Itoa(item.UnreadPriority),
			strconv.Itoa(item.AutoApply),
			strconv.Itoa(item.Review),
			strconv.Itoa(item.ReviewWithReason),
			strconv.Itoa(item.Hold),
			strconv.Itoa(item.Claude),
			strconv.Itoa(item.Blocklist),
		}); err != nil {
			return nil, err
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

func MarshalDailyLogsJSON(entries []types.ClassificationLogEntry, exportedAt time.Time) ([]byte, error) {
	return marshalIndented(exportEnvelope[dailyLogSummary]{
		ExportedAt: exportedAt.Format(time.RFC3339),
		Items:      buildDailyLogSummaries(entries),
	})
}

func marshalIndented(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}

	return append(data, '\n'), nil
}

func normalizeImportedBlocklist(items []blocklistJSONEntry) ([]types.UpsertBlocklistEntryRequest, error) {
	normalized := make([]types.UpsertBlocklistEntryRequest, 0, len(items))
	for _, item := range items {
		if !item.Kind.IsValid() {
			return nil, fmt.Errorf("blocklist kind %q は未対応です", item.Kind)
		}
		if strings.TrimSpace(item.Pattern) == "" {
			return nil, fmt.Errorf("blocklist pattern は必須です")
		}
		normalized = append(normalized, types.UpsertBlocklistEntryRequest{
			Kind:    item.Kind,
			Pattern: item.Pattern,
			Note:    item.Note,
		})
	}

	return normalized, nil
}

func filterImportant(entries []types.ClassificationLogEntry) []types.ClassificationLogEntry {
	filtered := make([]types.ClassificationLogEntry, 0)
	for _, entry := range entries {
		if entry.Category == types.ClassificationCategoryImportant {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func buildDailyLogSummaries(entries []types.ClassificationLogEntry) []dailyLogSummary {
	byDate := make(map[string]*dailyLogSummary)

	for _, entry := range entries {
		date := classifyDate(entry.ClassifiedAt)
		summary := byDate[date]
		if summary == nil {
			summary = &dailyLogSummary{Date: date}
			byDate[date] = summary
		}

		summary.Total++
		switch entry.Category {
		case types.ClassificationCategoryImportant:
			summary.Important++
		case types.ClassificationCategoryNewsletter:
			summary.Newsletter++
		case types.ClassificationCategoryJunk:
			summary.Junk++
		case types.ClassificationCategoryArchive:
			summary.Archive++
		case types.ClassificationCategoryUnreadPriority:
			summary.UnreadPriority++
		}

		switch entry.ReviewLevel {
		case types.ClassificationReviewLevelAutoApply:
			summary.AutoApply++
		case types.ClassificationReviewLevelReview:
			summary.Review++
		case types.ClassificationReviewLevelReviewWithReason:
			summary.ReviewWithReason++
		case types.ClassificationReviewLevelHold:
			summary.Hold++
		}

		switch entry.Source {
		case types.ClassificationSourceClaude:
			summary.Claude++
		case types.ClassificationSourceBlocklist:
			summary.Blocklist++
		}
	}

	keys := make([]string, 0, len(byDate))
	for key := range byDate {
		keys = append(keys, key)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(keys)))

	summaries := make([]dailyLogSummary, 0, len(keys))
	for _, key := range keys {
		summaries = append(summaries, *byDate[key])
	}

	return summaries
}

func classifyDate(value string) string {
	if value == "" {
		return ""
	}

	parsed, err := time.Parse(time.RFC3339, value)
	if err == nil {
		return parsed.Format("2006-01-02")
	}

	if len(value) >= 10 {
		return value[:10]
	}
	return value
}

func compactText(value string, limit int) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(value, "\n", " "))
	runes := []rune(trimmed)
	if len(runes) <= limit {
		return trimmed
	}
	return string(runes[:limit]) + "..."
}

func formatConfidence(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}
