package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
)

const (
	defaultKeychainService = "dev.mairu.desktop"
	googleTokenAccount     = "google-oauth-token"
	claudeAPIKeyAccount    = "claude-api-key"
)

var (
	// ErrSecretNotFound は要求した機密情報がキーチェーンに存在しないことを表す。
	ErrSecretNotFound = errors.New("機密情報が見つかりません")
	// ErrSecretStoreUnavailable は OS キーチェーンに接続できないことを表す。
	ErrSecretStoreUnavailable = errors.New("OS キーチェーンが利用できません")
)

// SecretStore は機密情報の保存先を抽象化する。
type SecretStore interface {
	SetSecret(ctx context.Context, account string, value []byte) error
	GetSecret(ctx context.Context, account string) ([]byte, error)
	DeleteSecret(ctx context.Context, account string) error
}

// SecretManager は OAuth トークンと Claude API キーの保存を扱う。
type SecretManager struct {
	store SecretStore
}

// NewSecretManager は機密情報管理を初期化する。
func NewSecretManager(store SecretStore) *SecretManager {
	if store == nil {
		panic("nil SecretStore passed to NewSecretManager")
	}

	return &SecretManager{store: store}
}

// NewSystemSecretStore は現在の OS に応じたキーチェーン実装を返す。
func NewSystemSecretStore() SecretStore {
	return newKeychainStore(defaultKeychainService)
}

// SaveGoogleToken は OAuth トークン一式を保存する。
func (m *SecretManager) SaveGoogleToken(ctx context.Context, token TokenSet) error {
	if strings.TrimSpace(token.AccessToken) == "" {
		return errors.New("アクセストークンが空です")
	}

	payload, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("OAuth トークンの直列化に失敗しました: %w", err)
	}

	if err := m.store.SetSecret(ctx, googleTokenAccount, payload); err != nil {
		return fmt.Errorf("OAuth トークンの保存に失敗しました: %w", err)
	}

	return nil
}

// LoadGoogleToken は保存済み OAuth トークンを取得する。
func (m *SecretManager) LoadGoogleToken(ctx context.Context) (TokenSet, error) {
	payload, err := m.store.GetSecret(ctx, googleTokenAccount)
	if err != nil {
		return TokenSet{}, err
	}

	var token TokenSet
	if err := json.Unmarshal(payload, &token); err != nil {
		return TokenSet{}, fmt.Errorf("保存済み OAuth トークンの復元に失敗しました: %w", err)
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return TokenSet{}, errors.New("保存済み OAuth トークンにアクセストークンがありません")
	}

	return token, nil
}

// HasGoogleToken は保存済み OAuth トークンの有無を返す。
func (m *SecretManager) HasGoogleToken(ctx context.Context) (bool, error) {
	token, err := m.LoadGoogleToken(ctx)
	if err == nil {
		return strings.TrimSpace(token.AccessToken) != "", nil
	}
	if errors.Is(err, ErrSecretNotFound) {
		return false, nil
	}
	return false, err
}

// SaveClaudeAPIKey は Claude API キーを保存する。
func (m *SecretManager) SaveClaudeAPIKey(ctx context.Context, apiKey string) error {
	trimmed := strings.TrimSpace(apiKey)
	if trimmed == "" {
		return errors.New("Claude API キーを入力してください")
	}

	if err := m.store.SetSecret(ctx, claudeAPIKeyAccount, []byte(trimmed)); err != nil {
		return fmt.Errorf("Claude API キーの保存に失敗しました: %w", err)
	}

	return nil
}

// LoadClaudeAPIKey は保存済み Claude API キーを取得する。
func (m *SecretManager) LoadClaudeAPIKey(ctx context.Context) (string, error) {
	payload, err := m.store.GetSecret(ctx, claudeAPIKeyAccount)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

// HasClaudeAPIKey は保存済み Claude API キーの有無を返す。
func (m *SecretManager) HasClaudeAPIKey(ctx context.Context) (bool, error) {
	_, err := m.store.GetSecret(ctx, claudeAPIKeyAccount)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrSecretNotFound) {
		return false, nil
	}
	return false, err
}

// DeleteClaudeAPIKey は保存済み Claude API キーを削除する。
func (m *SecretManager) DeleteClaudeAPIKey(ctx context.Context) error {
	err := m.store.DeleteSecret(ctx, claudeAPIKeyAccount)
	if err == nil || errors.Is(err, ErrSecretNotFound) {
		return nil
	}
	return fmt.Errorf("Claude API キーの削除に失敗しました: %w", err)
}

// MaskSecret は UI やログで機密値を直接出さないための短いプレビューを返す。
func MaskSecret(secret string) string {
	trimmed := strings.TrimSpace(secret)
	switch {
	case trimmed == "":
		return ""
	case len(trimmed) <= 8:
		return strings.Repeat("*", len(trimmed))
	default:
		return trimmed[:4] + strings.Repeat("*", 4) + trimmed[len(trimmed)-4:]
	}
}

// MemorySecretStore はテスト用のインメモリ実装。
type MemorySecretStore struct {
	mu      sync.RWMutex
	secrets map[string][]byte
}

// NewMemorySecretStore はテスト用ストアを初期化する。
func NewMemorySecretStore() *MemorySecretStore {
	return &MemorySecretStore{
		secrets: make(map[string][]byte),
	}
}

// SetSecret は機密情報を保持する。
func (s *MemorySecretStore) SetSecret(_ context.Context, account string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.secrets[account] = append([]byte(nil), value...)
	return nil
}

// GetSecret は機密情報を取り出す。
func (s *MemorySecretStore) GetSecret(_ context.Context, account string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	value, ok := s.secrets[account]
	if !ok {
		return nil, ErrSecretNotFound
	}

	return append([]byte(nil), value...), nil
}

// DeleteSecret は機密情報を削除する。
func (s *MemorySecretStore) DeleteSecret(_ context.Context, account string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.secrets[account]; !ok {
		return ErrSecretNotFound
	}

	delete(s.secrets, account)
	return nil
}
