package db

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mairu/internal/types"
)

func TestOpenCreatesSchemaAndEnablesWAL(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mairu.db")

	store, err := Open(ctx, OpenOptions{Path: path})
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	})

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("database file was not created: %v", err)
	}

	snapshot, err := store.HealthSnapshot(ctx)
	if err != nil {
		t.Fatalf("HealthSnapshot returned error: %v", err)
	}
	if !snapshot.Ready {
		t.Fatalf("HealthSnapshot.Ready = false, want true")
	}
	if snapshot.Path != path {
		t.Fatalf("HealthSnapshot.Path = %q, want %q", snapshot.Path, path)
	}
	if got := strings.ToLower(snapshot.JournalMode); got != "wal" {
		t.Fatalf("HealthSnapshot.JournalMode = %q, want wal", snapshot.JournalMode)
	}
	if snapshot.SchemaVersion != len(migrations) {
		t.Fatalf("HealthSnapshot.SchemaVersion = %d, want %d", snapshot.SchemaVersion, len(migrations))
	}

	for _, tableName := range []string{
		"schema_migrations",
		"blocklist",
		"action_logs",
		"settings",
		"classification_corrections",
	} {
		var found string
		err := store.db.QueryRowContext(
			ctx,
			`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`,
			tableName,
		).Scan(&found)
		if err != nil {
			t.Fatalf("table lookup for %q returned error: %v", tableName, err)
		}
		if found != tableName {
			t.Fatalf("table lookup for %q = %q, want %q", tableName, found, tableName)
		}
	}
}

func TestStorePersistsSettingsAcrossReopen(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mairu.db")

	store, err := Open(ctx, OpenOptions{Path: path})
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}

	if err := store.SetSetting(ctx, "last_run_at", "2026-03-03T10:00:00Z"); err != nil {
		t.Fatalf("SetSetting returned error: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	reopened, err := Open(ctx, OpenOptions{Path: path})
	if err != nil {
		t.Fatalf("Open after close returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Fatalf("Close after reopen returned error: %v", err)
		}
	})

	got, ok, err := reopened.GetSetting(ctx, "last_run_at")
	if err != nil {
		t.Fatalf("GetSetting returned error: %v", err)
	}
	if !ok {
		t.Fatalf("GetSetting ok = false, want true")
	}
	if got != "2026-03-03T10:00:00Z" {
		t.Fatalf("GetSetting = %q, want %q", got, "2026-03-03T10:00:00Z")
	}

	var applied int
	if err := reopened.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&applied); err != nil {
		t.Fatalf("schema_migrations count returned error: %v", err)
	}
	if applied != len(migrations) {
		t.Fatalf("schema_migrations count = %d, want %d", applied, len(migrations))
	}
}

func TestSetSettingsRollsBackOnError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mairu.db")

	store, err := Open(ctx, OpenOptions{Path: path})
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	})

	if err := store.SetSetting(ctx, "scheduler.enabled", "true"); err != nil {
		t.Fatalf("SetSetting returned error: %v", err)
	}

	err = store.SetSettings(ctx, map[string]string{
		"scheduler.enabled": "false",
		"   ":               "invalid",
	})
	if err == nil {
		t.Fatalf("SetSettings error = nil, want non-nil")
	}

	got, ok, err := store.GetSetting(ctx, "scheduler.enabled")
	if err != nil {
		t.Fatalf("GetSetting returned error: %v", err)
	}
	if !ok {
		t.Fatalf("GetSetting ok = false, want true")
	}
	if got != "true" {
		t.Fatalf("GetSetting = %q, want %q", got, "true")
	}
}

func TestBlocklistCRUD(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mairu.db")

	store, err := Open(ctx, OpenOptions{Path: path})
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	})

	sender, err := store.UpsertBlocklistEntry(ctx, types.BlocklistKindSender, "News <NEWS@Example.COM>", "テスト")
	if err != nil {
		t.Fatalf("UpsertBlocklistEntry sender returned error: %v", err)
	}
	if sender.Pattern != "news@example.com" {
		t.Fatalf("sender.Pattern = %q, want %q", sender.Pattern, "news@example.com")
	}

	domain, err := store.UpsertBlocklistEntry(ctx, types.BlocklistKindDomain, "@Example.com", "ドメイン")
	if err != nil {
		t.Fatalf("UpsertBlocklistEntry domain returned error: %v", err)
	}
	if domain.Pattern != "example.com" {
		t.Fatalf("domain.Pattern = %q, want %q", domain.Pattern, "example.com")
	}

	items, err := store.ListBlocklistEntries(ctx)
	if err != nil {
		t.Fatalf("ListBlocklistEntries returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("ListBlocklistEntries length = %d, want 2", len(items))
	}

	deleted, err := store.DeleteBlocklistEntry(ctx, sender.ID)
	if err != nil {
		t.Fatalf("DeleteBlocklistEntry returned error: %v", err)
	}
	if !deleted {
		t.Fatalf("DeleteBlocklistEntry deleted = false, want true")
	}

	items, err = store.ListBlocklistEntries(ctx)
	if err != nil {
		t.Fatalf("ListBlocklistEntries after delete returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("ListBlocklistEntries after delete length = %d, want 1", len(items))
	}
	if items[0].ID != domain.ID {
		t.Fatalf("remaining ID = %d, want %d", items[0].ID, domain.ID)
	}
}

func TestBlocklistSuggestionsFromCorrections(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mairu.db")

	store, err := Open(ctx, OpenOptions{Path: path})
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	})

	for i := 0; i < 3; i++ {
		err := store.RecordClassificationCorrection(ctx, types.ClassificationCorrection{
			MessageID:         "m-sender",
			Sender:            "promo@example.com",
			OriginalCategory:  types.ClassificationCategoryNewsletter,
			CorrectedCategory: types.ClassificationCategoryJunk,
		})
		if err != nil {
			t.Fatalf("RecordClassificationCorrection sender returned error: %v", err)
		}
	}

	for i := 0; i < 3; i++ {
		err := store.RecordClassificationCorrection(ctx, types.ClassificationCorrection{
			MessageID:         "m-domain",
			Sender:            "noreply@bulk.example.net",
			OriginalCategory:  types.ClassificationCategoryArchive,
			CorrectedCategory: types.ClassificationCategoryJunk,
		})
		if err != nil {
			t.Fatalf("RecordClassificationCorrection domain returned error: %v", err)
		}
	}
	err = store.RecordClassificationCorrection(ctx, types.ClassificationCorrection{
		MessageID:         "m-domain-2",
		Sender:            "offers@bulk.example.net",
		OriginalCategory:  types.ClassificationCategoryArchive,
		CorrectedCategory: types.ClassificationCategoryJunk,
	})
	if err != nil {
		t.Fatalf("RecordClassificationCorrection second domain sender returned error: %v", err)
	}

	// 既に sender が登録済みの場合は提案から除外されること。
	if _, err := store.UpsertBlocklistEntry(ctx, types.BlocklistKindSender, "promo@example.com", "already"); err != nil {
		t.Fatalf("UpsertBlocklistEntry returned error: %v", err)
	}

	suggestions, err := store.ListBlocklistSuggestions(ctx, 3)
	if err != nil {
		t.Fatalf("ListBlocklistSuggestions returned error: %v", err)
	}

	foundDomain := false
	for _, suggestion := range suggestions {
		if suggestion.Kind == types.BlocklistKindSender && suggestion.Pattern == "promo@example.com" {
			t.Fatalf("blocked sender should be excluded from suggestions: %+v", suggestion)
		}
		if suggestion.Kind == types.BlocklistKindDomain && suggestion.Pattern == "bulk.example.net" {
			foundDomain = true
			if suggestion.Count != 4 {
				t.Fatalf("bulk.example.net Count = %d, want 4", suggestion.Count)
			}
		}
	}
	if !foundDomain {
		t.Fatalf("bulk.example.net domain suggestion was not found: %+v", suggestions)
	}
}

func TestBlocklistSuggestionsDomainRequiresMultipleSenders(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mairu.db")

	store, err := Open(ctx, OpenOptions{Path: path})
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	})

	for i := 0; i < 4; i++ {
		err := store.RecordClassificationCorrection(ctx, types.ClassificationCorrection{
			MessageID:         "m-single-domain",
			Sender:            "single@domain-only.example",
			OriginalCategory:  types.ClassificationCategoryArchive,
			CorrectedCategory: types.ClassificationCategoryJunk,
		})
		if err != nil {
			t.Fatalf("RecordClassificationCorrection returned error: %v", err)
		}
	}

	suggestions, err := store.ListBlocklistSuggestions(ctx, 3)
	if err != nil {
		t.Fatalf("ListBlocklistSuggestions returned error: %v", err)
	}

	for _, suggestion := range suggestions {
		if suggestion.Kind == types.BlocklistKindDomain && suggestion.Pattern == "domain-only.example" {
			t.Fatalf("single-sender domain must not be suggested: %+v", suggestion)
		}
	}
}

func TestImportBlocklistEntries(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mairu.db")

	store, err := Open(ctx, OpenOptions{Path: path})
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	})

	imported, err := store.ImportBlocklistEntries(ctx, []types.UpsertBlocklistEntryRequest{
		{Kind: types.BlocklistKindSender, Pattern: "News <NEWS@example.com>", Note: "sender"},
		{Kind: types.BlocklistKindDomain, Pattern: "@example.org", Note: "domain"},
		{Kind: types.BlocklistKindSender, Pattern: "news@example.com", Note: "updated"},
	})
	if err != nil {
		t.Fatalf("ImportBlocklistEntries returned error: %v", err)
	}
	if imported != 3 {
		t.Fatalf("ImportBlocklistEntries imported = %d, want 3", imported)
	}

	items, err := store.ListBlocklistEntries(ctx)
	if err != nil {
		t.Fatalf("ListBlocklistEntries returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("ListBlocklistEntries length = %d, want 2", len(items))
	}

	foundUpdatedSender := false
	for _, item := range items {
		if item.Kind == types.BlocklistKindSender && item.Note == "updated" {
			foundUpdatedSender = true
		}
	}
	if !foundUpdatedSender {
		t.Fatalf("sender note was not updated: %+v", items)
	}
}

func TestClassificationAndActionLogs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mairu.db")

	store, err := Open(ctx, OpenOptions{Path: path})
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	})

	if err := store.RecordClassificationLogEntries(ctx, []types.ClassificationLogEntry{
		{
			MessageID:   "m-1",
			ThreadID:    "t-1",
			From:        "alerts@example.com",
			Subject:     "重要なお知らせ",
			Snippet:     "確認が必要です",
			Category:    types.ClassificationCategoryImportant,
			Confidence:  0.93,
			ReviewLevel: types.ClassificationReviewLevelAutoApply,
			Source:      types.ClassificationSourceClaude,
		},
		{
			MessageID:   "m-2",
			ThreadID:    "t-2",
			From:        "promo@example.com",
			Subject:     "ニュースレター",
			Snippet:     "今週の更新",
			Category:    types.ClassificationCategoryNewsletter,
			Confidence:  0.72,
			ReviewLevel: types.ClassificationReviewLevelReview,
			Source:      types.ClassificationSourceClaude,
		},
	}); err != nil {
		t.Fatalf("RecordClassificationLogEntries returned error: %v", err)
	}

	logs, err := store.ListClassificationLogEntries(ctx)
	if err != nil {
		t.Fatalf("ListClassificationLogEntries returned error: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("ListClassificationLogEntries length = %d, want 2", len(logs))
	}

	if err := store.RecordActionLogEntries(ctx, []types.ActionLogEntry{
		{
			MessageID:   "m-1",
			ThreadID:    "t-1",
			From:        "alerts@example.com",
			Subject:     "重要なお知らせ",
			ActionKind:  types.ActionKindLabel,
			Status:      "success",
			Detail:      "",
			Category:    types.ClassificationCategoryImportant,
			Confidence:  0.93,
			ReviewLevel: types.ClassificationReviewLevelAutoApply,
			Source:      types.ClassificationSourceClaude,
		},
		{
			MessageID:   "m-2",
			ThreadID:    "t-2",
			From:        "promo@example.com",
			Subject:     "ニュースレター",
			ActionKind:  types.ActionKindDelete,
			Status:      "failed",
			Detail:      "quota error",
			Category:    types.ClassificationCategoryJunk,
			Confidence:  0.88,
			ReviewLevel: types.ClassificationReviewLevelReview,
			Source:      types.ClassificationSourceBlocklist,
		},
	}); err != nil {
		t.Fatalf("RecordActionLogEntries returned error: %v", err)
	}

	actionLogs, err := store.ListActionLogEntries(ctx)
	if err != nil {
		t.Fatalf("ListActionLogEntries returned error: %v", err)
	}
	if len(actionLogs) != 2 {
		t.Fatalf("ListActionLogEntries length = %d, want 2", len(actionLogs))
	}
	if actionLogs[0].MessageID == "" || actionLogs[0].CreatedAt == "" {
		t.Fatalf("stored action log is missing required fields: %+v", actionLogs[0])
	}

	latest, ok, err := store.GetLatestActionLogEntry(ctx, "m-2", types.ActionKindDelete)
	if err != nil {
		t.Fatalf("GetLatestActionLogEntry returned error: %v", err)
	}
	if !ok {
		t.Fatalf("GetLatestActionLogEntry ok = false, want true")
	}
	if latest.Status != "failed" {
		t.Fatalf("latest.Status = %q, want %q", latest.Status, "failed")
	}
	if latest.Detail != "quota error" {
		t.Fatalf("latest.Detail = %q, want %q", latest.Detail, "quota error")
	}

	_, ok, err = store.GetLatestActionLogEntry(ctx, "missing", types.ActionKindDelete)
	if err != nil {
		t.Fatalf("GetLatestActionLogEntry(missing) returned error: %v", err)
	}
	if ok {
		t.Fatalf("GetLatestActionLogEntry(missing) ok = true, want false")
	}
}
