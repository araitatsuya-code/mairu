//go:build darwin && !cgo

package auth

import (
	"context"
	"fmt"
)

type unsupportedKeychainStore struct {
	reason string
}

func newKeychainStore(_ string) SecretStore {
	return unsupportedKeychainStore{reason: "CGO が無効なため macOS キーチェーンを利用できません"}
}

func (s unsupportedKeychainStore) SetSecret(_ context.Context, _ string, _ []byte) error {
	return fmt.Errorf("%w: %s", ErrSecretStoreUnavailable, s.reason)
}

func (s unsupportedKeychainStore) GetSecret(_ context.Context, _ string) ([]byte, error) {
	return nil, fmt.Errorf("%w: %s", ErrSecretStoreUnavailable, s.reason)
}

func (s unsupportedKeychainStore) DeleteSecret(_ context.Context, _ string) error {
	return fmt.Errorf("%w: %s", ErrSecretStoreUnavailable, s.reason)
}
