package auth

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestExchangeCode(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method: got %s, want %s", r.Method, http.MethodPost)
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll returned error: %v", err)
			}

			values, err := url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("ParseQuery returned error: %v", err)
			}

			assertTokenFormValue(t, values, "client_id", "client-id")
			assertTokenFormValue(t, values, "client_secret", "client-secret")
			assertTokenFormValue(t, values, "code", "code-123")
			assertTokenFormValue(t, values, "code_verifier", "verifier-456")
			assertTokenFormValue(t, values, "grant_type", "authorization_code")
			assertTokenFormValue(t, values, "redirect_uri", "http://127.0.0.1:8080/oauth2/callback")

			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				Body: io.NopCloser(strings.NewReader(`{
					"access_token":"access-token",
					"refresh_token":"refresh-token",
					"token_type":"Bearer",
					"scope":"scope-a scope-b",
					"expires_in":3600
				}`)),
			}, nil
		}),
	}

	client := NewClient(Config{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		TokenURL:     "https://example.test/token",
		HTTPClient:   httpClient,
	})

	before := time.Now().UTC()
	token, err := client.ExchangeCode(context.Background(), LoginResult{
		LoginSession: LoginSession{
			RedirectURL: "http://127.0.0.1:8080/oauth2/callback",
		},
		Code:         "code-123",
		CodeVerifier: "verifier-456",
	})
	if err != nil {
		t.Fatalf("ExchangeCode returned error: %v", err)
	}

	if token.AccessToken != "access-token" {
		t.Fatalf("AccessToken mismatch: got %q, want %q", token.AccessToken, "access-token")
	}
	if token.RefreshToken != "refresh-token" {
		t.Fatalf("RefreshToken mismatch: got %q, want %q", token.RefreshToken, "refresh-token")
	}
	if token.TokenType != "Bearer" {
		t.Fatalf("TokenType mismatch: got %q, want %q", token.TokenType, "Bearer")
	}
	if token.Scope != "scope-a scope-b" {
		t.Fatalf("Scope mismatch: got %q, want %q", token.Scope, "scope-a scope-b")
	}
	if token.Expiry.Before(before.Add(59 * time.Minute)) {
		t.Fatalf("Expiry looks too early: got %s", token.Expiry)
	}
}

func TestEnsureValidTokenReturnsStoredTokenWhenStillFresh(t *testing.T) {
	t.Parallel()

	client := NewClient(Config{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				t.Fatalf("HTTP request should not be sent for a fresh token: %s", r.URL.String())
				return nil, nil
			}),
		},
	})

	current := TokenSet{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		Expiry:       time.Now().UTC().Add(10 * time.Minute),
	}

	got, refreshed, err := client.EnsureValidToken(context.Background(), current)
	if err != nil {
		t.Fatalf("EnsureValidToken returned error: %v", err)
	}
	if refreshed {
		t.Fatalf("refreshed = true, want false")
	}
	if got != current {
		t.Fatalf("token mismatch: got %#v, want %#v", got, current)
	}
}

func TestEnsureValidTokenRefreshesExpiredToken(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method: got %s, want %s", r.Method, http.MethodPost)
			}
			if r.URL.String() != "https://example.test/token" {
				t.Fatalf("unexpected URL: got %s", r.URL.String())
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll returned error: %v", err)
			}

			values, err := url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("ParseQuery returned error: %v", err)
			}

			assertTokenFormValue(t, values, "client_id", "client-id")
			assertTokenFormValue(t, values, "client_secret", "client-secret")
			assertTokenFormValue(t, values, "refresh_token", "refresh-token")
			assertTokenFormValue(t, values, "grant_type", "refresh_token")

			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				Body: io.NopCloser(strings.NewReader(`{
					"access_token":"next-access-token",
					"token_type":"Bearer",
					"expires_in":3600
				}`)),
			}, nil
		}),
	}

	client := NewClient(Config{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		TokenURL:     "https://example.test/token",
		HTTPClient:   httpClient,
	})

	current := TokenSet{
		AccessToken:  "expired-access-token",
		RefreshToken: "refresh-token",
		TokenType:    "Bearer",
		Scope:        "scope-a scope-b",
		Expiry:       time.Now().UTC().Add(-time.Minute),
	}

	got, refreshed, err := client.EnsureValidToken(context.Background(), current)
	if err != nil {
		t.Fatalf("EnsureValidToken returned error: %v", err)
	}
	if !refreshed {
		t.Fatalf("refreshed = false, want true")
	}
	if got.AccessToken != "next-access-token" {
		t.Fatalf("AccessToken mismatch: got %q, want %q", got.AccessToken, "next-access-token")
	}
	if got.RefreshToken != current.RefreshToken {
		t.Fatalf("RefreshToken mismatch: got %q, want %q", got.RefreshToken, current.RefreshToken)
	}
	if got.Scope != current.Scope {
		t.Fatalf("Scope mismatch: got %q, want %q", got.Scope, current.Scope)
	}
	if !got.Expiry.After(time.Now().UTC().Add(59 * time.Minute)) {
		t.Fatalf("Expiry looks too early: got %s", got.Expiry)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func assertTokenFormValue(t *testing.T, values url.Values, key string, want string) {
	t.Helper()

	got := values.Get(key)
	if got != want {
		t.Fatalf("%s mismatch: got %q, want %q", key, got, want)
	}
}
