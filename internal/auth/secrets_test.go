package auth

import (
	"context"
	"testing"
	"time"
)

func TestSecretManagerGoogleTokenRoundTrip(t *testing.T) {
	t.Parallel()

	manager := NewSecretManager(NewMemorySecretStore())
	want := TokenSet{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		TokenType:    "Bearer",
		Scope:        "scope-a scope-b",
		Expiry:       time.Unix(1_700_000_000, 0).UTC(),
	}

	if err := manager.SaveGoogleToken(context.Background(), want); err != nil {
		t.Fatalf("SaveGoogleToken returned error: %v", err)
	}

	got, err := manager.LoadGoogleToken(context.Background())
	if err != nil {
		t.Fatalf("LoadGoogleToken returned error: %v", err)
	}

	if got != want {
		t.Fatalf("token mismatch: got %#v, want %#v", got, want)
	}
}

func TestSecretManagerClaudeAPIKeyLifecycle(t *testing.T) {
	t.Parallel()

	manager := NewSecretManager(NewMemorySecretStore())

	if err := manager.SaveClaudeAPIKey(context.Background(), "  secret-key  "); err != nil {
		t.Fatalf("SaveClaudeAPIKey returned error: %v", err)
	}

	stored, err := manager.LoadClaudeAPIKey(context.Background())
	if err != nil {
		t.Fatalf("LoadClaudeAPIKey returned error: %v", err)
	}
	if stored != "secret-key" {
		t.Fatalf("stored key mismatch: got %q, want %q", stored, "secret-key")
	}

	ok, err := manager.HasClaudeAPIKey(context.Background())
	if err != nil {
		t.Fatalf("HasClaudeAPIKey returned error: %v", err)
	}
	if !ok {
		t.Fatalf("HasClaudeAPIKey = false, want true")
	}

	if err := manager.DeleteClaudeAPIKey(context.Background()); err != nil {
		t.Fatalf("DeleteClaudeAPIKey returned error: %v", err)
	}

	ok, err = manager.HasClaudeAPIKey(context.Background())
	if err != nil {
		t.Fatalf("HasClaudeAPIKey after delete returned error: %v", err)
	}
	if ok {
		t.Fatalf("HasClaudeAPIKey after delete = true, want false")
	}
}

func TestMaskSecret(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: ""},
		{name: "short", input: "secret", want: "******"},
		{name: "long", input: "abcd1234wxyz", want: "abcd****wxyz"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := MaskSecret(tt.input)
			if got != tt.want {
				t.Fatalf("MaskSecret(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
