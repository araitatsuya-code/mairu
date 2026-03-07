package exporter

import (
	"strings"
	"testing"
	"time"

	"mairu/internal/types"
)

func TestMarshalAndParseBlocklistJSON(t *testing.T) {
	t.Parallel()

	data, err := MarshalBlocklistJSON([]types.BlocklistEntry{
		{
			ID:        1,
			Kind:      types.BlocklistKindSender,
			Pattern:   "news@example.com",
			Note:      "sender",
			CreatedAt: "2026-03-07T10:00:00Z",
			UpdatedAt: "2026-03-07T10:00:00Z",
		},
	}, time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("MarshalBlocklistJSON returned error: %v", err)
	}

	entries, err := ParseBlocklistJSON(data)
	if err != nil {
		t.Fatalf("ParseBlocklistJSON returned error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ParseBlocklistJSON length = %d, want 1", len(entries))
	}
	if entries[0].Pattern != "news@example.com" {
		t.Fatalf("pattern = %q, want news@example.com", entries[0].Pattern)
	}
}

func TestMarshalDailyLogsCSV(t *testing.T) {
	t.Parallel()

	data, err := MarshalDailyLogsCSV([]types.ClassificationLogEntry{
		{
			MessageID:    "m-1",
			Category:     types.ClassificationCategoryImportant,
			ReviewLevel:  types.ClassificationReviewLevelAutoApply,
			Source:       types.ClassificationSourceClaude,
			ClassifiedAt: "2026-03-07T10:00:00Z",
		},
		{
			MessageID:    "m-2",
			Category:     types.ClassificationCategoryJunk,
			ReviewLevel:  types.ClassificationReviewLevelReview,
			Source:       types.ClassificationSourceBlocklist,
			ClassifiedAt: "2026-03-07T12:00:00Z",
		},
	})
	if err != nil {
		t.Fatalf("MarshalDailyLogsCSV returned error: %v", err)
	}

	text := string(data)
	if !strings.Contains(text, "2026-03-07,2,1,0,1") {
		t.Fatalf("daily log csv does not contain expected summary: %s", text)
	}
}

func TestMarshalImportantSummaryPDF(t *testing.T) {
	t.Parallel()

	data, err := MarshalImportantSummaryPDF([]types.ClassificationLogEntry{
		{
			MessageID:    "m-1",
			From:         "alerts@example.com",
			Subject:      "重要なお知らせ",
			Snippet:      "確認が必要です",
			Category:     types.ClassificationCategoryImportant,
			Confidence:   0.95,
			ReviewLevel:  types.ClassificationReviewLevelAutoApply,
			Source:       types.ClassificationSourceClaude,
			ClassifiedAt: "2026-03-07T10:00:00Z",
		},
	}, time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("MarshalImportantSummaryPDF returned error: %v", err)
	}
	if !strings.HasPrefix(string(data[:8]), "%PDF-1.4") {
		t.Fatalf("pdf header mismatch: %q", string(data[:8]))
	}
}
