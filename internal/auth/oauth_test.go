package auth

import (
	"net/url"
	"testing"
)

func TestBuildCodeChallenge(t *testing.T) {
	t.Parallel()

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	got := buildCodeChallenge(verifier)
	want := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

	if got != want {
		t.Fatalf("code challenge mismatch: got %q, want %q", got, want)
	}
}

func TestBuildAuthorizationURL(t *testing.T) {
	t.Parallel()

	client := NewClient(Config{
		ClientID: "client-id-123",
		Scopes: []string{
			"scope-a",
			"scope-b",
		},
	})

	rawURL, err := client.buildAuthorizationURL(
		"http://127.0.0.1:58432/oauth2/callback",
		"state-123",
		"challenge-456",
	)
	if err != nil {
		t.Fatalf("buildAuthorizationURL returned error: %v", err)
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("url.Parse returned error: %v", err)
	}

	query := parsed.Query()
	assertQueryValue(t, query, "client_id", "client-id-123")
	assertQueryValue(t, query, "redirect_uri", "http://127.0.0.1:58432/oauth2/callback")
	assertQueryValue(t, query, "response_type", "code")
	assertQueryValue(t, query, "scope", "scope-a scope-b")
	assertQueryValue(t, query, "state", "state-123")
	assertQueryValue(t, query, "code_challenge", "challenge-456")
	assertQueryValue(t, query, "code_challenge_method", "S256")
	assertQueryValue(t, query, "access_type", "offline")
	assertQueryValue(t, query, "include_granted_scopes", "true")
	assertQueryValue(t, query, "prompt", "consent")
}

func TestClientIsConfigured(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config Config
		want   bool
	}{
		{
			name: "client id and secret",
			config: Config{
				ClientID:     "client-id",
				ClientSecret: "client-secret",
			},
			want: true,
		},
		{
			name: "missing secret",
			config: Config{
				ClientID: "client-id",
			},
			want: false,
		},
		{
			name: "missing id",
			config: Config{
				ClientSecret: "client-secret",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := NewClient(tt.config)
			if got := client.IsConfigured(); got != tt.want {
				t.Fatalf("IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func assertQueryValue(t *testing.T, values url.Values, key string, want string) {
	t.Helper()

	got := values.Get(key)
	if got != want {
		t.Fatalf("%s mismatch: got %q, want %q", key, got, want)
	}
}
