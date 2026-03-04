package db

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	for _, tableName := range []string{"schema_migrations", "blocklist", "action_logs", "settings"} {
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
