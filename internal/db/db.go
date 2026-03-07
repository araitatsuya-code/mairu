package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"os"
	"path/filepath"
	"strings"

	"mairu/internal/types"

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
	{
		version: 2,
		name:    "create classification corrections",
		statements: []string{
			`CREATE TABLE IF NOT EXISTS classification_corrections (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				message_id TEXT NOT NULL DEFAULT '',
				sender_email TEXT NOT NULL,
				sender_domain TEXT NOT NULL DEFAULT '',
				original_category TEXT NOT NULL,
				corrected_category TEXT NOT NULL,
				created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
			)`,
			`CREATE INDEX IF NOT EXISTS idx_classification_corrections_sender_email
				ON classification_corrections(sender_email, created_at DESC)`,
			`CREATE INDEX IF NOT EXISTS idx_classification_corrections_sender_domain
				ON classification_corrections(sender_domain, created_at DESC)`,
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

// ListBlocklistEntries は登録済みブロックリストを一覧で返す。
func (s *Store) ListBlocklistEntries(ctx context.Context) ([]types.BlocklistEntry, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, kind, pattern, note, created_at, updated_at
		FROM blocklist
		ORDER BY updated_at DESC, id DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("blocklist 一覧を取得できませんでした: %w", err)
	}
	defer rows.Close()

	items := make([]types.BlocklistEntry, 0)
	for rows.Next() {
		var item types.BlocklistEntry
		if err := rows.Scan(
			&item.ID,
			&item.Kind,
			&item.Pattern,
			&item.Note,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("blocklist を読み取れませんでした: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("blocklist 走査に失敗しました: %w", err)
	}

	return items, nil
}

// UpsertBlocklistEntry は sender/domain ブロックを追加または更新する。
func (s *Store) UpsertBlocklistEntry(
	ctx context.Context,
	kind types.BlocklistKind,
	pattern string,
	note string,
) (types.BlocklistEntry, error) {
	if err := s.ensureReady(); err != nil {
		return types.BlocklistEntry{}, err
	}

	normalizedPattern, err := normalizeBlockPattern(kind, pattern)
	if err != nil {
		return types.BlocklistEntry{}, err
	}

	trimmedNote := strings.TrimSpace(note)
	if _, err := s.db.ExecContext(
		ctx,
		`INSERT INTO blocklist (kind, pattern, note, created_at, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(kind, pattern) DO UPDATE SET
			note = excluded.note,
			updated_at = CURRENT_TIMESTAMP`,
		kind,
		normalizedPattern,
		trimmedNote,
	); err != nil {
		return types.BlocklistEntry{}, fmt.Errorf("blocklist を保存できませんでした: %w", err)
	}

	var item types.BlocklistEntry
	err = s.db.QueryRowContext(
		ctx,
		`SELECT id, kind, pattern, note, created_at, updated_at
		FROM blocklist
		WHERE kind = ? AND pattern = ?`,
		kind,
		normalizedPattern,
	).Scan(
		&item.ID,
		&item.Kind,
		&item.Pattern,
		&item.Note,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return types.BlocklistEntry{}, fmt.Errorf("保存後の blocklist を取得できませんでした: %w", err)
	}

	return item, nil
}

// DeleteBlocklistEntry は ID 指定でブロックリストを削除する。
func (s *Store) DeleteBlocklistEntry(ctx context.Context, id int64) (bool, error) {
	if err := s.ensureReady(); err != nil {
		return false, err
	}
	if id <= 0 {
		return false, errors.New("blocklist の ID は 1 以上で指定してください")
	}

	result, err := s.db.ExecContext(ctx, `DELETE FROM blocklist WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("blocklist を削除できませんでした: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("削除結果を確認できませんでした: %w", err)
	}
	return affected > 0, nil
}

// RecordClassificationCorrection は分類修正履歴を保存する。
func (s *Store) RecordClassificationCorrection(
	ctx context.Context,
	correction types.ClassificationCorrection,
) error {
	if err := s.ensureReady(); err != nil {
		return err
	}
	if !correction.OriginalCategory.IsValid() {
		return errors.New("original_category が不正です")
	}
	if !correction.CorrectedCategory.IsValid() {
		return errors.New("corrected_category が不正です")
	}

	senderEmail := normalizeSenderAddress(correction.Sender)
	if senderEmail == "" {
		return errors.New("sender は有効なメールアドレスを含めてください")
	}
	senderDomain := senderDomain(senderEmail)
	if senderDomain == "" {
		return errors.New("sender からドメインを抽出できませんでした")
	}

	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO classification_corrections (
			message_id,
			sender_email,
			sender_domain,
			original_category,
			corrected_category
		) VALUES (?, ?, ?, ?, ?)`,
		strings.TrimSpace(correction.MessageID),
		senderEmail,
		senderDomain,
		correction.OriginalCategory,
		correction.CorrectedCategory,
	)
	if err != nil {
		return fmt.Errorf("分類修正履歴を保存できませんでした: %w", err)
	}

	return nil
}

// ListBlocklistSuggestions は修正履歴からブロック候補を返す。
func (s *Store) ListBlocklistSuggestions(
	ctx context.Context,
	minCount int,
) ([]types.BlocklistSuggestion, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}

	threshold := minCount
	if threshold < 1 {
		threshold = 1
	}

	suggestions := make([]types.BlocklistSuggestion, 0)

	senderRows, err := s.db.QueryContext(
		ctx,
		`SELECT c.sender_email, COUNT(*) AS hit_count, MAX(c.created_at) AS last_seen_at
		FROM classification_corrections c
		WHERE c.corrected_category = ?
			AND c.sender_email <> ''
			AND NOT EXISTS (
				SELECT 1
				FROM blocklist b
				WHERE b.kind = ? AND b.pattern = c.sender_email
			)
		GROUP BY c.sender_email
		HAVING hit_count >= ?
		ORDER BY hit_count DESC, last_seen_at DESC`,
		types.ClassificationCategoryJunk,
		types.BlocklistKindSender,
		threshold,
	)
	if err != nil {
		return nil, fmt.Errorf("送信者提案を取得できませんでした: %w", err)
	}
	defer senderRows.Close()

	for senderRows.Next() {
		var pattern string
		var count int
		var lastSeenAt string
		if err := senderRows.Scan(&pattern, &count, &lastSeenAt); err != nil {
			return nil, fmt.Errorf("送信者提案を読み取れませんでした: %w", err)
		}
		suggestions = append(suggestions, types.BlocklistSuggestion{
			Kind:        types.BlocklistKindSender,
			Pattern:     pattern,
			Count:       count,
			LastSeenAt:  lastSeenAt,
			Description: fmt.Sprintf("同一送信者を junk へ %d 回修正", count),
		})
	}
	if err := senderRows.Err(); err != nil {
		return nil, fmt.Errorf("送信者提案の走査に失敗しました: %w", err)
	}

	domainRows, err := s.db.QueryContext(
		ctx,
		`SELECT
			c.sender_domain,
			COUNT(*) AS hit_count,
			COUNT(DISTINCT c.sender_email) AS unique_sender_count,
			MAX(c.created_at) AS last_seen_at
		FROM classification_corrections c
		WHERE c.corrected_category = ?
			AND c.sender_domain <> ''
			AND NOT EXISTS (
				SELECT 1
				FROM blocklist b
				WHERE b.kind = ? AND b.pattern = c.sender_domain
			)
		GROUP BY c.sender_domain
		HAVING hit_count >= ? AND unique_sender_count >= 2
		ORDER BY hit_count DESC, last_seen_at DESC`,
		types.ClassificationCategoryJunk,
		types.BlocklistKindDomain,
		threshold,
	)
	if err != nil {
		return nil, fmt.Errorf("ドメイン提案を取得できませんでした: %w", err)
	}
	defer domainRows.Close()

	for domainRows.Next() {
		var pattern string
		var count int
		var uniqueSenders int
		var lastSeenAt string
		if err := domainRows.Scan(&pattern, &count, &uniqueSenders, &lastSeenAt); err != nil {
			return nil, fmt.Errorf("ドメイン提案を読み取れませんでした: %w", err)
		}
		suggestions = append(suggestions, types.BlocklistSuggestion{
			Kind:        types.BlocklistKindDomain,
			Pattern:     pattern,
			Count:       count,
			LastSeenAt:  lastSeenAt,
			Description: fmt.Sprintf("同一ドメインを junk へ %d 回修正 (%d 送信者)", count, uniqueSenders),
		})
	}
	if err := domainRows.Err(); err != nil {
		return nil, fmt.Errorf("ドメイン提案の走査に失敗しました: %w", err)
	}

	return suggestions, nil
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

func normalizeBlockPattern(kind types.BlocklistKind, pattern string) (string, error) {
	if !kind.IsValid() {
		return "", errors.New("blocklist kind は sender か domain を指定してください")
	}

	normalized := strings.ToLower(strings.TrimSpace(pattern))
	if normalized == "" {
		return "", errors.New("blocklist pattern は必須です")
	}

	switch kind {
	case types.BlocklistKindSender:
		normalized = normalizeSenderAddress(normalized)
		if normalized == "" {
			return "", errors.New("sender は有効なメールアドレスを含めてください")
		}
		return normalized, nil
	case types.BlocklistKindDomain:
		normalized = strings.TrimPrefix(normalized, "@")
		if at := strings.LastIndex(normalized, "@"); at >= 0 {
			normalized = normalized[at+1:]
		}
		normalized = strings.TrimSpace(normalized)
		if normalized == "" || strings.Contains(normalized, " ") {
			return "", errors.New("domain は有効なドメインを指定してください")
		}
		return normalized, nil
	default:
		return "", errors.New("blocklist kind は sender か domain を指定してください")
	}
}

func normalizeSenderAddress(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return ""
	}

	if parsed, err := mail.ParseAddress(trimmed); err == nil {
		return strings.TrimSpace(strings.ToLower(parsed.Address))
	}

	if strings.Contains(trimmed, "<") && strings.Contains(trimmed, ">") {
		start := strings.Index(trimmed, "<")
		end := strings.LastIndex(trimmed, ">")
		if start >= 0 && end > start+1 {
			candidate := strings.TrimSpace(trimmed[start+1 : end])
			if strings.Contains(candidate, "@") && !strings.Contains(candidate, " ") {
				return candidate
			}
		}
	}

	if strings.Count(trimmed, "@") == 1 && !strings.Contains(trimmed, " ") {
		return trimmed
	}

	return ""
}

func senderDomain(email string) string {
	address := normalizeSenderAddress(email)
	at := strings.LastIndex(address, "@")
	if at < 0 || at+1 >= len(address) {
		return ""
	}
	return address[at+1:]
}
