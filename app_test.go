package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"mairu/internal/auth"
	"mairu/internal/claude"
	"mairu/internal/gmail"
	"mairu/internal/types"
)

func TestGetRuntimeStatusIncludesStoredSecretPreviews(t *testing.T) {
	t.Parallel()

	store := auth.NewMemorySecretStore()
	manager := auth.NewSecretManager(store)
	if err := manager.SaveGoogleToken(context.Background(), auth.TokenSet{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
	}); err != nil {
		t.Fatalf("SaveGoogleToken returned error: %v", err)
	}
	if err := manager.SaveClaudeAPIKey(context.Background(), "claude-secret-key"); err != nil {
		t.Fatalf("SaveClaudeAPIKey returned error: %v", err)
	}

	app := &App{
		authClient:    auth.NewClient(auth.Config{ClientID: "client-id", ClientSecret: "client-secret"}),
		secretManager: manager,
	}

	status := app.GetRuntimeStatus()

	if !status.Authorized {
		t.Fatalf("Authorized = false, want true")
	}
	if status.GoogleTokenPreview != "refr****oken" {
		t.Fatalf("GoogleTokenPreview = %q, want %q", status.GoogleTokenPreview, "refr****oken")
	}
	if !status.ClaudeConfigured {
		t.Fatalf("ClaudeConfigured = false, want true")
	}
	if status.ClaudeKeyPreview != "clau****-key" {
		t.Fatalf("ClaudeKeyPreview = %q, want %q", status.ClaudeKeyPreview, "clau****-key")
	}
}

func TestGetRuntimeStatusFallsBackToAccessTokenPreview(t *testing.T) {
	t.Parallel()

	store := auth.NewMemorySecretStore()
	manager := auth.NewSecretManager(store)
	if err := manager.SaveGoogleToken(context.Background(), auth.TokenSet{
		AccessToken: "access-token",
	}); err != nil {
		t.Fatalf("SaveGoogleToken returned error: %v", err)
	}

	app := &App{
		authClient:    auth.NewClient(auth.Config{ClientID: "client-id", ClientSecret: "client-secret"}),
		secretManager: manager,
	}

	status := app.GetRuntimeStatus()

	if status.GoogleTokenPreview != "acce****oken" {
		t.Fatalf("GoogleTokenPreview = %q, want %q", status.GoogleTokenPreview, "acce****oken")
	}
}

func TestCheckGmailConnectionRefreshesStoredToken(t *testing.T) {
	t.Parallel()

	store := auth.NewMemorySecretStore()
	manager := auth.NewSecretManager(store)
	if err := manager.SaveGoogleToken(context.Background(), auth.TokenSet{
		AccessToken:  "expired-access-token",
		RefreshToken: "refresh-token",
		Expiry:       time.Now().UTC().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("SaveGoogleToken returned error: %v", err)
	}

	httpClient := &http.Client{
		Transport: appRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.String() {
			case "https://oauth.test/token":
				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: io.NopCloser(strings.NewReader(`{
						"access_token":"fresh-access-token",
						"expires_in":3600
					}`)),
				}, nil
			case "https://gmail.test/gmail/v1/users/me/profile":
				if got := r.Header.Get("Authorization"); got != "Bearer fresh-access-token" {
					t.Fatalf("Authorization mismatch: got %q", got)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: io.NopCloser(strings.NewReader(`{
						"emailAddress":"user@example.com",
						"messagesTotal":10,
						"threadsTotal":5,
						"historyId":"12345"
					}`)),
				}, nil
			default:
				t.Fatalf("unexpected URL: %s", r.URL.String())
				return nil, nil
			}
		}),
	}

	app := &App{
		authClient: auth.NewClient(auth.Config{
			ClientID:     "client-id",
			ClientSecret: "client-secret",
			TokenURL:     "https://oauth.test/token",
			HTTPClient:   httpClient,
		}),
		gmailClient:   gmail.NewClient(gmail.Options{BaseURL: "https://gmail.test", HTTPClient: httpClient}),
		secretManager: manager,
	}

	result := app.CheckGmailConnection()
	if !result.Success {
		t.Fatalf("CheckGmailConnection failed: %s", result.Message)
	}
	if !result.TokenRefreshed {
		t.Fatalf("TokenRefreshed = false, want true")
	}
	if result.EmailAddress != "user@example.com" {
		t.Fatalf("EmailAddress = %q, want %q", result.EmailAddress, "user@example.com")
	}

	stored, err := manager.LoadGoogleToken(context.Background())
	if err != nil {
		t.Fatalf("LoadGoogleToken returned error: %v", err)
	}
	if stored.AccessToken != "fresh-access-token" {
		t.Fatalf("stored AccessToken = %q, want %q", stored.AccessToken, "fresh-access-token")
	}
}

func TestGetRuntimeStatusClearsGmailSuccessWhenUnauthorized(t *testing.T) {
	t.Parallel()

	app := &App{
		authClient:     auth.NewClient(auth.Config{ClientID: "client-id", ClientSecret: "client-secret"}),
		secretManager:  auth.NewSecretManager(auth.NewMemorySecretStore()),
		gmailStatus:    buildGmailConnectedStatusMessage("user@example.com"),
		gmailConnected: true,
		gmailAccount:   "user@example.com",
	}

	status := app.GetRuntimeStatus()

	if status.GmailConnected {
		t.Fatalf("GmailConnected = true, want false")
	}
	if status.GmailStatus != buildBlockedGmailStatusMessage() {
		t.Fatalf("GmailStatus = %q, want %q", status.GmailStatus, buildBlockedGmailStatusMessage())
	}
	if status.GmailAccountEmail != "" {
		t.Fatalf("GmailAccountEmail = %q, want empty", status.GmailAccountEmail)
	}
}

func TestClassifyEmailsUsesStoredClaudeAPIKey(t *testing.T) {
	t.Parallel()

	store := auth.NewMemorySecretStore()
	manager := auth.NewSecretManager(store)
	if err := manager.SaveClaudeAPIKey(context.Background(), "claude-secret"); err != nil {
		t.Fatalf("SaveClaudeAPIKey returned error: %v", err)
	}

	httpClient := &http.Client{
		Transport: appRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != "https://claude.test/v1/messages" {
				t.Fatalf("unexpected URL: %s", r.URL.String())
			}
			if got := r.Header.Get("x-api-key"); got != "claude-secret" {
				t.Fatalf("x-api-key mismatch: got %q", got)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				Body: io.NopCloser(strings.NewReader(`{
					"content":[
						{
							"type":"text",
							"text":"[{\"id\":\"msg-1\",\"category\":\"important\",\"confidence\":0.92,\"reason\":\"返信が必要です\"}]"
						}
					]
				}`)),
			}, nil
		}),
	}

	app := &App{
		claudeClient: claude.NewClient(claude.Options{
			BaseURL:      "https://claude.test",
			DefaultModel: "claude-test-model",
			HTTPClient:   httpClient,
		}),
		secretManager: manager,
	}

	result, err := app.ClassifyEmails(types.ClassificationRequest{
		Messages: []types.EmailSummary{
			{
				ID:      "msg-1",
				From:    "boss@example.com",
				Subject: "至急",
				Snippet: "確認してください",
				Unread:  true,
			},
		},
	})
	if err != nil {
		t.Fatalf("ClassifyEmails returned error: %v", err)
	}

	if result.Model != "claude-test-model" {
		t.Fatalf("Model = %q, want %q", result.Model, "claude-test-model")
	}
	if len(result.Results) != 1 {
		t.Fatalf("Results length = %d, want 1", len(result.Results))
	}
	if result.Results[0].ReviewLevel != types.ClassificationReviewLevelAutoApply {
		t.Fatalf("ReviewLevel = %q, want %q", result.Results[0].ReviewLevel, types.ClassificationReviewLevelAutoApply)
	}
}

type appRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn appRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}
