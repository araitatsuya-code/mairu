package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

const driverName = "sqlite"

// Store は SQLite ベースの永続化ストアを保持する。
type Store struct {
	db   *sql.DB
	path string
}

// OpenOptions は DB 初期化時の基本設定を表す。
type OpenOptions struct {
	Path    string
	AppName string
}

// HealthSnapshot は DB 初期化状況の簡易確認に使う。
type HealthSnapshot struct {
	Ready         bool
	Path          string
	JournalMode   string
	SchemaVersion int
}

type migration struct {
	version    int
	name       string
	statements []string
}

var migrations = []migration{
	{
		version: 1,
		name:    "create base tables",
		statements: []string{
			`CREATE TABLE IF NOT EXISTS blocklist (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				kind TEXT NOT NULL,
				pattern TEXT NOT NULL,
				note TEXT NOT NULL DEFAULT '',
				created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
				UNIQUE(kind, pattern)
			)`,
			`CREATE TABLE IF NOT EXISTS action_logs (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				message_id TEXT NOT NULL,
				thread_id TEXT NOT NULL DEFAULT '',
				action_kind TEXT NOT NULL,
				status TEXT NOT NULL,
				detail TEXT NOT NULL DEFAULT '',
				created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
			)`,
			`CREATE TABLE IF NOT EXISTS settings (
				key TEXT PRIMARY KEY,
				value TEXT NOT NULL,
				updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
			)`,
			`CREATE INDEX IF NOT EXISTS idx_blocklist_kind_pattern ON blocklist(kind, pattern)`,
			`CREATE INDEX IF NOT EXISTS idx_action_logs_created_at ON action_logs(created_at DESC)`,
		},
	},
}

// Open は SQLite を初期化し、必要なマイグレーションを適用した Store を返す。
func Open(ctx context.Context, options OpenOptions) (*Store, error) {
	path, err := resolvePath(options)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("DB ディレクトリを作成できませんでした: %w", err)
	}

	handle, err := sql.Open(driverName, path)
	if err != nil {
		return nil, fmt.Errorf("SQLite を開けませんでした: %w", err)
	}
	handle.SetMaxOpenConns(1)
	handle.SetMaxIdleConns(1)

	if err := handle.PingContext(ctx); err != nil {
		handle.Close()
		return nil, fmt.Errorf("SQLite 接続確認に失敗しました: %w", err)
	}

	store := &Store{
		db:   handle,
		path: path,
	}

	if err := store.configure(ctx); err != nil {
		handle.Close()
		return nil, err
	}
	if err := store.applyMigrations(ctx); err != nil {
		handle.Close()
		return nil, err
	}

	return store, nil
}

// DefaultPath はアプリ名に対応した標準の SQLite ファイルパスを返す。
func DefaultPath(appName string) (string, error) {
	baseDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("設定ディレクトリを取得できませんでした: %w", err)
	}

	name := strings.TrimSpace(appName)
	if name == "" {
		name = "Mairu"
	}

	return filepath.Join(baseDir, name, "mairu.db"), nil
}

// Close は DB 接続を閉じる。
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// HealthSnapshot は DB の状態を読み出す。
func (s *Store) HealthSnapshot(ctx context.Context) (HealthSnapshot, error) {
	if err := s.ensureReady(); err != nil {
		return HealthSnapshot{}, err
	}

	journalMode := ""
	if err := s.db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		return HealthSnapshot{}, fmt.Errorf("journal_mode を確認できませんでした: %w", err)
	}

	version, err := s.schemaVersion(ctx)
	if err != nil {
		return HealthSnapshot{}, err
	}

	return HealthSnapshot{
		Ready:         true,
		Path:          s.path,
		JournalMode:   journalMode,
		SchemaVersion: version,
	}, nil
}

// SetSetting は設定値をキー単位で保存する。
func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	if err := s.ensureReady(); err != nil {
		return err
	}

	normalizedKey := strings.TrimSpace(key)
	if normalizedKey == "" {
		return errors.New("settings のキーは必須です")
	}

	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO settings (key, value, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = CURRENT_TIMESTAMP`,
		normalizedKey,
		value,
	)
	if err != nil {
		return fmt.Errorf("settings を保存できませんでした: %w", err)
	}

	return nil
}

// GetSetting は設定値をキー単位で読み出す。
func (s *Store) GetSetting(ctx context.Context, key string) (string, bool, error) {
	if err := s.ensureReady(); err != nil {
		return "", false, err
	}

	normalizedKey := strings.TrimSpace(key)
	if normalizedKey == "" {
		return "", false, errors.New("settings のキーは必須です")
	}

	var value string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT value FROM settings WHERE key = ?`,
		normalizedKey,
	).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("settings を読み出せませんでした: %w", err)
	}

	return value, true, nil
}

func (s *Store) configure(ctx context.Context) error {
	if err := s.ensureReady(); err != nil {
		return err
	}

	if _, err := s.db.ExecContext(ctx, `PRAGMA busy_timeout = 5000`); err != nil {
		return fmt.Errorf("busy_timeout を設定できませんでした: %w", err)
	}

	journalMode := ""
	if err := s.db.QueryRowContext(ctx, `PRAGMA journal_mode = WAL`).Scan(&journalMode); err != nil {
		return fmt.Errorf("WAL モードを設定できませんでした: %w", err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		return fmt.Errorf("WAL モードを有効化できませんでした: got %q", journalMode)
	}

	if _, err := s.db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("foreign_keys を設定できませんでした: %w", err)
	}

	if _, err := s.db.ExecContext(ctx, `PRAGMA synchronous = NORMAL`); err != nil {
		return fmt.Errorf("synchronous を設定できませんでした: %w", err)
	}

	return nil
}

func (s *Store) applyMigrations(ctx context.Context) error {
	if err := s.ensureReady(); err != nil {
		return err
	}

	if err := s.ensureMigrationTable(ctx); err != nil {
		return err
	}

	applied, err := s.loadAppliedMigrations(ctx)
	if err != nil {
		return err
	}

	for _, item := range migrations {
		if applied[item.version] {
			continue
		}
		if err := s.applyMigration(ctx, item); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) ensureMigrationTable(ctx context.Context) error {
	_, err := s.db.ExecContext(
		ctx,
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
	)
	if err != nil {
		return fmt.Errorf("schema_migrations を作成できませんでした: %w", err)
	}

	return nil
}

func (s *Store) loadAppliedMigrations(ctx context.Context) (map[int]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("適用済み migration を取得できませんでした: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("migration version を読み取れませんでした: %w", err)
		}
		applied[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("適用済み migration の走査に失敗しました: %w", err)
	}

	return applied, nil
}

func (s *Store) applyMigration(ctx context.Context, item migration) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("migration %d を開始できませんでした: %w", item.version, err)
	}

	for _, statement := range item.statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d (%s) の適用に失敗しました: %w", item.version, item.name, err)
		}
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO schema_migrations (version, name) VALUES (?, ?)`,
		item.version,
		item.name,
	); err != nil {
		tx.Rollback()
		return fmt.Errorf("migration %d の記録に失敗しました: %w", item.version, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("migration %d を確定できませんでした: %w", item.version, err)
	}

	return nil
}

func (s *Store) schemaVersion(ctx context.Context) (int, error) {
	var version sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("schema version を取得できませんでした: %w", err)
	}
	if !version.Valid {
		return 0, nil
	}

	return int(version.Int64), nil
}

func (s *Store) ensureReady() error {
	if s == nil || s.db == nil {
		return errors.New("DB は初期化されていません")
	}
	return nil
}

func resolvePath(options OpenOptions) (string, error) {
	path := strings.TrimSpace(options.Path)
	if path != "" {
		return path, nil
	}

	return DefaultPath(options.AppName)
}
