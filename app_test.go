package main

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mairu/internal/auth"
	"mairu/internal/claude"
	"mairu/internal/db"
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

func TestClassifyEmailsSkipsBlockedSenderWithoutClaudeKey(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := db.Open(ctx, db.OpenOptions{
		Path: filepath.Join(t.TempDir(), "mairu.db"),
	})
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("store.Close returned error: %v", err)
		}
	})

	if _, err := store.UpsertBlocklistEntry(ctx, types.BlocklistKindSender, "promo@example.com", "manual"); err != nil {
		t.Fatalf("UpsertBlocklistEntry returned error: %v", err)
	}

	app := &App{
		secretManager: auth.NewSecretManager(auth.NewMemorySecretStore()),
		dbStore:       store,
		databaseReady: true,
	}

	result, err := app.ClassifyEmails(types.ClassificationRequest{
		Messages: []types.EmailSummary{
			{
				ID:      "msg-1",
				From:    "promo@example.com",
				Subject: "promo",
				Snippet: "sale",
				Unread:  true,
			},
		},
	})
	if err != nil {
		t.Fatalf("ClassifyEmails returned error: %v", err)
	}
	if result.Model != "blocklist-skip" {
		t.Fatalf("Model = %q, want %q", result.Model, "blocklist-skip")
	}
	if len(result.Results) != 1 {
		t.Fatalf("Results length = %d, want 1", len(result.Results))
	}
	if result.Results[0].Source != types.ClassificationSourceBlocklist {
		t.Fatalf("Source = %q, want %q", result.Results[0].Source, types.ClassificationSourceBlocklist)
	}
	if result.Results[0].Category != types.ClassificationCategoryJunk {
		t.Fatalf("Category = %q, want %q", result.Results[0].Category, types.ClassificationCategoryJunk)
	}
}

func TestClassifyEmailsCallsClaudeOnlyForUnblockedMessages(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := db.Open(ctx, db.OpenOptions{
		Path: filepath.Join(t.TempDir(), "mairu.db"),
	})
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("store.Close returned error: %v", err)
		}
	})

	if _, err := store.UpsertBlocklistEntry(ctx, types.BlocklistKindSender, "block@example.com", "manual"); err != nil {
		t.Fatalf("UpsertBlocklistEntry returned error: %v", err)
	}

	secretStore := auth.NewMemorySecretStore()
	manager := auth.NewSecretManager(secretStore)
	if err := manager.SaveClaudeAPIKey(ctx, "claude-secret"); err != nil {
		t.Fatalf("SaveClaudeAPIKey returned error: %v", err)
	}

	httpClient := &http.Client{
		Transport: appRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != "https://claude.test/v1/messages" {
				t.Fatalf("unexpected URL: %s", r.URL.String())
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll returned error: %v", err)
			}
			bodyText := string(body)
			if strings.Contains(bodyText, "msg-block") {
				t.Fatalf("blocked message was sent to Claude payload: %s", bodyText)
			}
			if !strings.Contains(bodyText, "msg-open") {
				t.Fatalf("unblocked message not found in payload: %s", bodyText)
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
							"text":"[{\"id\":\"msg-open\",\"category\":\"important\",\"confidence\":0.91,\"reason\":\"need action\"}]"
						}
					]
				}`)),
			}, nil
		}),
	}

	app := &App{
		secretManager: manager,
		claudeClient: claude.NewClient(claude.Options{
			BaseURL:      "https://claude.test",
			DefaultModel: "claude-test-model",
			HTTPClient:   httpClient,
		}),
		dbStore:       store,
		databaseReady: true,
	}

	result, err := app.ClassifyEmails(types.ClassificationRequest{
		Messages: []types.EmailSummary{
			{
				ID:      "msg-block",
				From:    "block@example.com",
				Subject: "promo",
				Snippet: "sale",
				Unread:  true,
			},
			{
				ID:      "msg-open",
				From:    "boss@example.com",
				Subject: "urgent",
				Snippet: "reply",
				Unread:  true,
			},
		},
	})
	if err != nil {
		t.Fatalf("ClassifyEmails returned error: %v", err)
	}
	if len(result.Results) != 2 {
		t.Fatalf("Results length = %d, want 2", len(result.Results))
	}
	if result.Results[0].MessageID != "msg-block" {
		t.Fatalf("first MessageID = %q, want %q", result.Results[0].MessageID, "msg-block")
	}
	if result.Results[0].Source != types.ClassificationSourceBlocklist {
		t.Fatalf("first Source = %q, want %q", result.Results[0].Source, types.ClassificationSourceBlocklist)
	}
	if result.Results[1].MessageID != "msg-open" {
		t.Fatalf("second MessageID = %q, want %q", result.Results[1].MessageID, "msg-open")
	}
	if result.Results[1].Source != types.ClassificationSourceClaude {
		t.Fatalf("second Source = %q, want %q", result.Results[1].Source, types.ClassificationSourceClaude)
	}
}

func TestExecuteGmailActionsRequiresConfirmation(t *testing.T) {
	t.Parallel()

	app := &App{}

	result, err := app.ExecuteGmailActions(types.ExecuteGmailActionsRequest{
		Confirmed: false,
		Decisions: []types.GmailActionDecision{
			{
				MessageID:   "msg-1",
				Category:    types.ClassificationCategoryJunk,
				ReviewLevel: types.ClassificationReviewLevelReview,
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteGmailActions returned error: %v", err)
	}
	if result.Success {
		t.Fatalf("Success = true, want false")
	}
	if !strings.Contains(result.Message, "確認ステップ") {
		t.Fatalf("Message = %q, want contains 確認ステップ", result.Message)
	}
}

func TestExecuteGmailActionsRefreshesToken(t *testing.T) {
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
			case "https://gmail.test/gmail/v1/users/me/messages/msg-1/trash":
				if got := r.Header.Get("Authorization"); got != "Bearer fresh-access-token" {
					t.Fatalf("Authorization mismatch: got %q", got)
				}
				return &http.Response{
					StatusCode: http.StatusNoContent,
					Header:     http.Header{},
					Body:       io.NopCloser(strings.NewReader("")),
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

	result, err := app.ExecuteGmailActions(types.ExecuteGmailActionsRequest{
		Confirmed: true,
		Decisions: []types.GmailActionDecision{
			{
				MessageID:   "msg-1",
				Category:    types.ClassificationCategoryJunk,
				ReviewLevel: types.ClassificationReviewLevelReview,
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteGmailActions returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, want true (message=%q)", result.Message)
	}
	if !result.TokenRefreshed {
		t.Fatalf("TokenRefreshed = false, want true")
	}
	if result.DeletedCount != 1 {
		t.Fatalf("DeletedCount = %d, want 1", result.DeletedCount)
	}

	stored, err := manager.LoadGoogleToken(context.Background())
	if err != nil {
		t.Fatalf("LoadGoogleToken returned error: %v", err)
	}
	if stored.AccessToken != "fresh-access-token" {
		t.Fatalf("stored AccessToken = %q, want %q", stored.AccessToken, "fresh-access-token")
	}
}

type appRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn appRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}
