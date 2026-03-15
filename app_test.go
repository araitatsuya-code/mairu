package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mairu/internal/auth"
	"mairu/internal/claude"
	"mairu/internal/db"
	"mairu/internal/gmail"
	"mairu/internal/gws"
	"mairu/internal/scheduler"
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

func TestGetRuntimeStatusIncludesGWSAvailability(t *testing.T) {
	t.Parallel()

	binaryPath := writeTestExecutableScript(t, `#!/bin/sh
echo "gws 0.9.0"
`)

	app := &App{
		authClient:    auth.NewClient(auth.Config{ClientID: "client-id", ClientSecret: "client-secret"}),
		secretManager: auth.NewSecretManager(auth.NewMemorySecretStore()),
		gwsClient:     gws.NewClient(gws.Options{BinaryPath: binaryPath}),
	}

	status := app.GetRuntimeStatus()
	if !status.GWSAvailable {
		t.Fatalf("GWSAvailable = false, want true")
	}
	if !strings.Contains(status.GWSStatus, binaryPath) {
		t.Fatalf("GWSStatus = %q, want path included", status.GWSStatus)
	}
}

func TestCheckGWSDiagnosticsReturnsResult(t *testing.T) {
	t.Parallel()

	binaryPath := writeTestExecutableScript(t, `#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "gws 0.9.0"
  exit 0
fi
echo "unexpected" >&2
exit 3
`)

	app := &App{
		gwsClient: gws.NewClient(gws.Options{BinaryPath: binaryPath}),
	}

	result := app.CheckGWSDiagnostics()
	if !result.Success {
		t.Fatalf("Success = false, want true (message=%q output=%q)", result.Message, result.Output)
	}
	if result.ErrorKind != types.GWSCLIErrorKindNone {
		t.Fatalf("ErrorKind = %q, want %q", result.ErrorKind, types.GWSCLIErrorKindNone)
	}
	if result.Version != "gws 0.9.0" {
		t.Fatalf("Version = %q, want %q", result.Version, "gws 0.9.0")
	}
}

func TestPreviewGWSGmailDryRunMapsInvalidCommand(t *testing.T) {
	t.Parallel()

	binaryPath := writeTestExecutableScript(t, `#!/bin/sh
echo "invalid command" >&2
exit 3
`)

	app := &App{
		gwsClient: gws.NewClient(gws.Options{BinaryPath: binaryPath}),
	}

	result := app.PreviewGWSGmailDryRun(types.GWSGmailDryRunRequest{
		Query:      "label:inbox",
		MaxResults: 10,
	})
	if result.Success {
		t.Fatalf("Success = true, want false")
	}
	if result.ErrorKind != types.GWSCLIErrorKindInvalidCommand {
		t.Fatalf("ErrorKind = %q, want %q", result.ErrorKind, types.GWSCLIErrorKindInvalidCommand)
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

func TestFetchClassificationMessagesUsesFetchConditions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := auth.NewMemorySecretStore()
	manager := auth.NewSecretManager(store)
	if err := manager.SaveGoogleToken(ctx, auth.TokenSet{AccessToken: "access-token"}); err != nil {
		t.Fatalf("SaveGoogleToken returned error: %v", err)
	}

	httpClient := &http.Client{
		Transport: appRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/gmail/v1/users/me/messages":
				if got := r.URL.Query().Get("maxResults"); got != "25" {
					t.Fatalf("maxResults = %q, want %q", got, "25")
				}
				if got := r.URL.Query().Get("q"); got != "newer_than:7d -category:promotions" {
					t.Fatalf("q = %q, want %q", got, "newer_than:7d -category:promotions")
				}
				if got := r.URL.Query().Get("pageToken"); got != "page-2" {
					t.Fatalf("pageToken = %q, want %q", got, "page-2")
				}
				gotLabelIDs := r.URL.Query()["labelIds"]
				if len(gotLabelIDs) != 2 {
					t.Fatalf("len(labelIds) = %d, want 2", len(gotLabelIDs))
				}
				if gotLabelIDs[0] != "INBOX" || gotLabelIDs[1] != "LabelImportant" {
					t.Fatalf("labelIds = %v, want [INBOX LabelImportant]", gotLabelIDs)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
					t.Fatalf("Authorization = %q, want %q", got, "Bearer access-token")
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: io.NopCloser(strings.NewReader(`{
						"messages":[{"id":"msg-1","threadId":"thread-1"}],
						"nextPageToken":"next-page"
					}`)),
				}, nil
			case "/gmail/v1/users/me/messages/msg-1":
				if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
					t.Fatalf("Authorization = %q, want %q", got, "Bearer access-token")
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: io.NopCloser(strings.NewReader(`{
						"id":"msg-1",
						"threadId":"thread-1",
						"snippet":"message snippet",
						"labelIds":["INBOX","UNREAD"],
						"payload":{"headers":[
							{"name":"From","value":"sender@example.com"},
							{"name":"Subject","value":"subject line"}
						]}
					}`)),
				}, nil
			default:
				t.Fatalf("unexpected URL: %s", r.URL.String())
				return nil, nil
			}
		}),
	}

	app := &App{
		authClient:    auth.NewClient(auth.Config{}),
		gmailClient:   gmail.NewClient(gmail.Options{BaseURL: "https://gmail.test", HTTPClient: httpClient}),
		secretManager: manager,
	}

	result, err := app.FetchClassificationMessages(types.FetchClassificationMessagesRequest{
		Query:      " newer_than:7d -category:promotions ",
		MaxResults: 25,
		LabelIDs:   []string{" INBOX ", "", "LabelImportant", "INBOX"},
		PageToken:  " page-2 ",
	})
	if err != nil {
		t.Fatalf("FetchClassificationMessages returned error: %v", err)
	}
	if result.TokenRefreshed {
		t.Fatalf("TokenRefreshed = true, want false")
	}
	if result.NextPageToken != "next-page" {
		t.Fatalf("NextPageToken = %q, want %q", result.NextPageToken, "next-page")
	}
	if len(result.Messages) != 1 {
		t.Fatalf("len(Messages) = %d, want 1", len(result.Messages))
	}
	if result.Messages[0].ID != "msg-1" {
		t.Fatalf("Messages[0].ID = %q, want %q", result.Messages[0].ID, "msg-1")
	}
	if !result.Messages[0].Unread {
		t.Fatalf("Messages[0].Unread = false, want true")
	}
}

func TestFetchClassificationMessagesRefreshesStoredToken(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := auth.NewMemorySecretStore()
	manager := auth.NewSecretManager(store)
	if err := manager.SaveGoogleToken(ctx, auth.TokenSet{
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
			case "https://gmail.test/gmail/v1/users/me/messages?maxResults=50":
				if got := r.Header.Get("Authorization"); got != "Bearer fresh-access-token" {
					t.Fatalf("Authorization = %q, want %q", got, "Bearer fresh-access-token")
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: io.NopCloser(strings.NewReader(`{"messages":[]}`)),
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

	result, err := app.FetchClassificationMessages(types.FetchClassificationMessagesRequest{})
	if err != nil {
		t.Fatalf("FetchClassificationMessages returned error: %v", err)
	}
	if !result.TokenRefreshed {
		t.Fatalf("TokenRefreshed = false, want true")
	}

	stored, err := manager.LoadGoogleToken(ctx)
	if err != nil {
		t.Fatalf("LoadGoogleToken returned error: %v", err)
	}
	if stored.AccessToken != "fresh-access-token" {
		t.Fatalf("stored AccessToken = %q, want %q", stored.AccessToken, "fresh-access-token")
	}
}

func TestListGmailLabelsReturnsLabels(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := auth.NewMemorySecretStore()
	manager := auth.NewSecretManager(store)
	if err := manager.SaveGoogleToken(ctx, auth.TokenSet{AccessToken: "access-token"}); err != nil {
		t.Fatalf("SaveGoogleToken returned error: %v", err)
	}

	httpClient := &http.Client{
		Transport: appRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != "https://gmail.test/gmail/v1/users/me/labels" {
				t.Fatalf("unexpected URL: %s", r.URL.String())
			}
			if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
				t.Fatalf("Authorization = %q, want %q", got, "Bearer access-token")
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				Body: io.NopCloser(strings.NewReader(`{
					"labels":[
						{"id":"LabelImportant","name":"Mairu/Important","type":"user"},
						{"id":"INBOX","name":"INBOX","type":"system"}
					]
				}`)),
			}, nil
		}),
	}

	app := &App{
		authClient:    auth.NewClient(auth.Config{}),
		gmailClient:   gmail.NewClient(gmail.Options{BaseURL: "https://gmail.test", HTTPClient: httpClient}),
		secretManager: manager,
	}

	result, err := app.ListGmailLabels()
	if err != nil {
		t.Fatalf("ListGmailLabels returned error: %v", err)
	}
	if result.TokenRefreshed {
		t.Fatalf("TokenRefreshed = true, want false")
	}
	if len(result.Labels) != 2 {
		t.Fatalf("len(Labels) = %d, want 2", len(result.Labels))
	}
	if result.Labels[0].ID != "INBOX" {
		t.Fatalf("Labels[0].ID = %q, want %q", result.Labels[0].ID, "INBOX")
	}
	if result.Labels[1].ID != "LabelImportant" {
		t.Fatalf("Labels[1].ID = %q, want %q", result.Labels[1].ID, "LabelImportant")
	}
}

func TestFetchGmailMessageDetailReturnsMessageDetail(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := auth.NewMemorySecretStore()
	manager := auth.NewSecretManager(store)
	if err := manager.SaveGoogleToken(ctx, auth.TokenSet{AccessToken: "access-token"}); err != nil {
		t.Fatalf("SaveGoogleToken returned error: %v", err)
	}

	httpClient := &http.Client{
		Transport: appRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != "https://gmail.test/gmail/v1/users/me/messages/msg-1?format=full" {
				t.Fatalf("unexpected URL: %s", r.URL.String())
			}
			if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
				t.Fatalf("Authorization = %q, want %q", got, "Bearer access-token")
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				Body: io.NopCloser(strings.NewReader(`{
					"id":"msg-1",
					"threadId":"thread-1",
					"snippet":"snippet text",
					"labelIds":["INBOX","UNREAD"],
					"payload":{
						"mimeType":"text/plain",
						"headers":[
							{"name":"From","value":"sender@example.com"},
							{"name":"To","value":"user@example.com"},
							{"name":"Subject","value":"subject line"}
						],
						"body":{"data":"SGVsbG8gcGxhaW4gYm9keQ"}
					}
				}`)),
			}, nil
		}),
	}

	app := &App{
		authClient:    auth.NewClient(auth.Config{}),
		gmailClient:   gmail.NewClient(gmail.Options{BaseURL: "https://gmail.test", HTTPClient: httpClient}),
		secretManager: manager,
	}

	result, err := app.FetchGmailMessageDetail("msg-1")
	if err != nil {
		t.Fatalf("FetchGmailMessageDetail returned error: %v", err)
	}
	if result.ID != "msg-1" {
		t.Fatalf("ID = %q, want %q", result.ID, "msg-1")
	}
	if result.ThreadID != "thread-1" {
		t.Fatalf("ThreadID = %q, want %q", result.ThreadID, "thread-1")
	}
	if result.Subject != "subject line" {
		t.Fatalf("Subject = %q, want %q", result.Subject, "subject line")
	}
	if result.BodyText != "Hello plain body" {
		t.Fatalf("BodyText = %q, want %q", result.BodyText, "Hello plain body")
	}
	if !result.Unread {
		t.Fatalf("Unread = false, want true")
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

func TestClassifyEmailsRejectsEmptyMessages(t *testing.T) {
	t.Parallel()

	store := auth.NewMemorySecretStore()
	manager := auth.NewSecretManager(store)
	if err := manager.SaveClaudeAPIKey(context.Background(), "claude-secret"); err != nil {
		t.Fatalf("SaveClaudeAPIKey returned error: %v", err)
	}

	app := &App{
		secretManager: manager,
		claudeClient:  claude.NewClient(claude.Options{}),
	}

	_, err := app.ClassifyEmails(types.ClassificationRequest{
		Messages: nil,
	})
	if err == nil {
		t.Fatalf("ClassifyEmails returned nil error, want error")
	}
	if !strings.Contains(err.Error(), "分類対象のメールがありません") {
		t.Fatalf("unexpected error: %v", err)
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

func TestClassifyEmailsSkipsBlockedDomainWithoutClaudeKey(t *testing.T) {
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

	if _, err := store.UpsertBlocklistEntry(ctx, types.BlocklistKindDomain, "example.com", "manual"); err != nil {
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
				From:    "noreply@example.com",
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

	secretStore := auth.NewMemorySecretStore()
	manager := auth.NewSecretManager(secretStore)
	if err := manager.SaveGoogleToken(context.Background(), auth.TokenSet{
		AccessToken:  "expired-access-token",
		RefreshToken: "refresh-token",
		Expiry:       time.Now().UTC().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("SaveGoogleToken returned error: %v", err)
	}

	ctx := context.Background()
	dbStore, err := db.Open(ctx, db.OpenOptions{
		Path: filepath.Join(t.TempDir(), "mairu.db"),
	})
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := dbStore.Close(); err != nil {
			t.Fatalf("dbStore.Close returned error: %v", err)
		}
	})

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
		dbStore:       dbStore,
		databaseReady: true,
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

func TestExecuteGmailActionsSkipsSucceededActionAndRecordsSkipLog(t *testing.T) {
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

	if err := store.RecordActionLogEntries(ctx, []types.ActionLogEntry{
		{
			MessageID:   "msg-1",
			ThreadID:    "thread-1",
			From:        "sender1@example.com",
			Subject:     "subject-1",
			ActionKind:  types.ActionKindDelete,
			Status:      actionLogStatusSuccess,
			Category:    types.ClassificationCategoryJunk,
			Confidence:  0.98,
			ReviewLevel: types.ClassificationReviewLevelAutoApply,
			Source:      types.ClassificationSourceClaude,
		},
	}); err != nil {
		t.Fatalf("RecordActionLogEntries returned error: %v", err)
	}

	secretStore := auth.NewMemorySecretStore()
	manager := auth.NewSecretManager(secretStore)
	if err := manager.SaveGoogleToken(ctx, auth.TokenSet{AccessToken: "access-token"}); err != nil {
		t.Fatalf("SaveGoogleToken returned error: %v", err)
	}

	requestCount := 0
	httpClient := &http.Client{
		Transport: appRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			requestCount++
			if r.URL.Path != "/gmail/v1/users/me/messages/msg-2/trash" {
				t.Fatalf("unexpected Gmail URL path: %s", r.URL.Path)
			}
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}),
	}

	app := &App{
		authClient:    auth.NewClient(auth.Config{}),
		gmailClient:   gmail.NewClient(gmail.Options{BaseURL: "https://gmail.test", HTTPClient: httpClient}),
		secretManager: manager,
		dbStore:       store,
		databaseReady: true,
	}

	result, err := app.ExecuteGmailActions(types.ExecuteGmailActionsRequest{
		Confirmed: true,
		Decisions: []types.GmailActionDecision{
			{
				MessageID:   "msg-1",
				Category:    types.ClassificationCategoryJunk,
				ReviewLevel: types.ClassificationReviewLevelAutoApply,
			},
			{
				MessageID:   "msg-2",
				Category:    types.ClassificationCategoryJunk,
				ReviewLevel: types.ClassificationReviewLevelAutoApply,
			},
		},
		Metadata: []types.GmailActionMetadata{
			{
				MessageID:   "msg-1",
				ThreadID:    "thread-1",
				From:        "sender1@example.com",
				Subject:     "subject-1",
				Category:    types.ClassificationCategoryJunk,
				Confidence:  0.98,
				ReviewLevel: types.ClassificationReviewLevelAutoApply,
				Source:      types.ClassificationSourceClaude,
			},
			{
				MessageID:   "msg-2",
				ThreadID:    "thread-2",
				From:        "sender2@example.com",
				Subject:     "subject-2",
				Category:    types.ClassificationCategoryJunk,
				Confidence:  0.93,
				ReviewLevel: types.ClassificationReviewLevelAutoApply,
				Source:      types.ClassificationSourceClaude,
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteGmailActions returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, want true (message=%q)", result.Message)
	}
	if result.ProcessedCount != 2 {
		t.Fatalf("ProcessedCount = %d, want 2", result.ProcessedCount)
	}
	if result.SuccessCount != 1 {
		t.Fatalf("SuccessCount = %d, want 1", result.SuccessCount)
	}
	if result.SkippedCount != 1 {
		t.Fatalf("SkippedCount = %d, want 1", result.SkippedCount)
	}
	if !strings.Contains(result.Message, "スキップ 1 件") {
		t.Fatalf("Message = %q, want contains skip count", result.Message)
	}
	if requestCount != 1 {
		t.Fatalf("requestCount = %d, want 1", requestCount)
	}

	latestSkipped, ok, err := store.GetLatestActionLogEntry(ctx, "msg-1", types.ActionKindDelete)
	if err != nil {
		t.Fatalf("GetLatestActionLogEntry(msg-1) returned error: %v", err)
	}
	if !ok {
		t.Fatalf("GetLatestActionLogEntry(msg-1) ok = false, want true")
	}
	if latestSkipped.Status != actionLogStatusSuccess {
		t.Fatalf("latestSkipped.Status = %q, want %q", latestSkipped.Status, actionLogStatusSuccess)
	}
	if !strings.Contains(latestSkipped.Detail, "重複防止") {
		t.Fatalf("latestSkipped.Detail = %q, want contains duplicate-skip reason", latestSkipped.Detail)
	}

	latestExecuted, ok, err := store.GetLatestActionLogEntry(ctx, "msg-2", types.ActionKindDelete)
	if err != nil {
		t.Fatalf("GetLatestActionLogEntry(msg-2) returned error: %v", err)
	}
	if !ok {
		t.Fatalf("GetLatestActionLogEntry(msg-2) ok = false, want true")
	}
	if latestExecuted.Status != actionLogStatusSuccess {
		t.Fatalf("latestExecuted.Status = %q, want %q", latestExecuted.Status, actionLogStatusSuccess)
	}
}

func TestExecuteGmailActionsRetriesFailedAndPendingActionLogs(t *testing.T) {
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

	if err := store.RecordActionLogEntries(ctx, []types.ActionLogEntry{
		{
			MessageID:   "msg-failed",
			ActionKind:  types.ActionKindDelete,
			Status:      actionLogStatusFailed,
			Detail:      "temporary error",
			Category:    types.ClassificationCategoryJunk,
			Confidence:  0.8,
			ReviewLevel: types.ClassificationReviewLevelAutoApply,
			Source:      types.ClassificationSourceClaude,
		},
		{
			MessageID:   "msg-pending",
			ActionKind:  types.ActionKindDelete,
			Status:      actionLogStatusPending,
			Detail:      "in progress",
			Category:    types.ClassificationCategoryJunk,
			Confidence:  0.8,
			ReviewLevel: types.ClassificationReviewLevelAutoApply,
			Source:      types.ClassificationSourceClaude,
		},
	}); err != nil {
		t.Fatalf("RecordActionLogEntries returned error: %v", err)
	}

	secretStore := auth.NewMemorySecretStore()
	manager := auth.NewSecretManager(secretStore)
	if err := manager.SaveGoogleToken(ctx, auth.TokenSet{AccessToken: "access-token"}); err != nil {
		t.Fatalf("SaveGoogleToken returned error: %v", err)
	}

	requestCount := 0
	httpClient := &http.Client{
		Transport: appRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			requestCount++
			if r.URL.Path != "/gmail/v1/users/me/messages/msg-failed/trash" &&
				r.URL.Path != "/gmail/v1/users/me/messages/msg-pending/trash" {
				t.Fatalf("unexpected Gmail URL path: %s", r.URL.Path)
			}
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}),
	}

	app := &App{
		authClient:    auth.NewClient(auth.Config{}),
		gmailClient:   gmail.NewClient(gmail.Options{BaseURL: "https://gmail.test", HTTPClient: httpClient}),
		secretManager: manager,
		dbStore:       store,
		databaseReady: true,
	}

	result, err := app.ExecuteGmailActions(types.ExecuteGmailActionsRequest{
		Confirmed: true,
		Decisions: []types.GmailActionDecision{
			{
				MessageID:   "msg-failed",
				Category:    types.ClassificationCategoryJunk,
				ReviewLevel: types.ClassificationReviewLevelAutoApply,
			},
			{
				MessageID:   "msg-pending",
				Category:    types.ClassificationCategoryJunk,
				ReviewLevel: types.ClassificationReviewLevelAutoApply,
			},
		},
		Metadata: []types.GmailActionMetadata{
			{
				MessageID:   "msg-failed",
				Category:    types.ClassificationCategoryJunk,
				Confidence:  0.8,
				ReviewLevel: types.ClassificationReviewLevelAutoApply,
				Source:      types.ClassificationSourceClaude,
			},
			{
				MessageID:   "msg-pending",
				Category:    types.ClassificationCategoryJunk,
				Confidence:  0.8,
				ReviewLevel: types.ClassificationReviewLevelAutoApply,
				Source:      types.ClassificationSourceClaude,
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteGmailActions returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, want true (message=%q)", result.Message)
	}
	if result.SuccessCount != 2 {
		t.Fatalf("SuccessCount = %d, want 2", result.SuccessCount)
	}
	if result.SkippedCount != 0 {
		t.Fatalf("SkippedCount = %d, want 0", result.SkippedCount)
	}
	if requestCount != 2 {
		t.Fatalf("requestCount = %d, want 2", requestCount)
	}
}

func TestGetRuntimeStatusReadsSchedulerLastRunAt(t *testing.T) {
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

	want := time.Date(2026, time.March, 7, 9, 30, 0, 0, time.UTC)
	if err := store.SetSetting(ctx, schedulerSettingLastRunAt, want.Format(time.RFC3339)); err != nil {
		t.Fatalf("SetSetting returned error: %v", err)
	}

	app := &App{
		authClient:    auth.NewClient(auth.Config{ClientID: "client-id", ClientSecret: "client-secret"}),
		secretManager: auth.NewSecretManager(auth.NewMemorySecretStore()),
		dbStore:       store,
		databaseReady: true,
	}

	status := app.GetRuntimeStatus()
	if status.LastRunAt == nil {
		t.Fatalf("LastRunAt = nil, want non-nil")
	}
	if !status.LastRunAt.Equal(want) {
		t.Fatalf("LastRunAt = %s, want %s", status.LastRunAt.UTC().Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func TestGetSchedulerSettingsReturnsDefaultsWhenDBUnavailable(t *testing.T) {
	t.Parallel()

	app := &App{}
	got := app.GetSchedulerSettings()

	if got.ClassificationIntervalMinutes != defaultClassificationIntervalMinutes {
		t.Fatalf("ClassificationIntervalMinutes = %d, want %d", got.ClassificationIntervalMinutes, defaultClassificationIntervalMinutes)
	}
	if got.BlocklistIntervalMinutes != defaultBlocklistUpdateMinutes {
		t.Fatalf("BlocklistIntervalMinutes = %d, want %d", got.BlocklistIntervalMinutes, defaultBlocklistUpdateMinutes)
	}
	if got.KnownBlockIntervalMinutes != defaultKnownBlockProcessMinutes {
		t.Fatalf("KnownBlockIntervalMinutes = %d, want %d", got.KnownBlockIntervalMinutes, defaultKnownBlockProcessMinutes)
	}
	if !got.NotificationsEnabled {
		t.Fatalf("NotificationsEnabled = false, want true")
	}
}

func TestUpdateSchedulerSettingsPersistsValues(t *testing.T) {
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

	app := &App{
		ctx:           ctx,
		dbStore:       store,
		databaseReady: true,
	}
	t.Cleanup(app.stopScheduler)

	result := app.UpdateSchedulerSettings(types.UpdateSchedulerSettingsRequest{
		ClassificationIntervalMinutes: 90,
		BlocklistIntervalMinutes:      120,
		KnownBlockIntervalMinutes:     15,
		NotificationsEnabled:          false,
	})
	if !result.Success {
		t.Fatalf("UpdateSchedulerSettings success = false, message=%q", result.Message)
	}

	saved := app.GetSchedulerSettings()
	if saved.ClassificationIntervalMinutes != 90 {
		t.Fatalf("ClassificationIntervalMinutes = %d, want 90", saved.ClassificationIntervalMinutes)
	}
	if saved.BlocklistIntervalMinutes != 120 {
		t.Fatalf("BlocklistIntervalMinutes = %d, want 120", saved.BlocklistIntervalMinutes)
	}
	if saved.KnownBlockIntervalMinutes != 15 {
		t.Fatalf("KnownBlockIntervalMinutes = %d, want 15", saved.KnownBlockIntervalMinutes)
	}
	if saved.NotificationsEnabled {
		t.Fatalf("NotificationsEnabled = true, want false")
	}

	if app.schedulerSvc == nil {
		t.Fatalf("schedulerSvc = nil, want non-nil")
	}
}

func TestBuildSchedulerNotificationWithPendingApproval(t *testing.T) {
	t.Parallel()

	notification, ok := buildSchedulerNotification(scheduler.Event{
		JobID: schedulerJobClassification,
		Kind:  scheduler.EventKindSucceeded,
		At:    time.Date(2026, time.March, 8, 12, 0, 0, 0, time.UTC),
		Result: scheduler.Result{
			Success:         5,
			Failed:          1,
			PendingApproval: 2,
			Message:         "分類を完了しました。",
		},
	})
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if notification.Level != "warning" {
		t.Fatalf("Level = %q, want %q", notification.Level, "warning")
	}
	if !strings.Contains(notification.Body, "承認待ち 2件") {
		t.Fatalf("Body = %q, want contains pending summary", notification.Body)
	}
}

func TestBuildSchedulerNotificationIncludesSkippedCount(t *testing.T) {
	t.Parallel()

	notification, ok := buildSchedulerNotification(scheduler.Event{
		JobID: schedulerJobClassification,
		Kind:  scheduler.EventKindSucceeded,
		Result: scheduler.Result{
			Processed:       10,
			Success:         5,
			Failed:          1,
			PendingApproval: 2,
			Message:         "分類を完了しました。",
		},
	})
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if !strings.Contains(notification.Body, "スキップ 2件") {
		t.Fatalf("Body = %q, want contains skipped summary", notification.Body)
	}
}

func TestBuildSchedulerNotificationSkipsPassiveSkip(t *testing.T) {
	t.Parallel()

	_, ok := buildSchedulerNotification(scheduler.Event{
		JobID: schedulerJobBlocklist,
		Kind:  scheduler.EventKindSkipped,
		Result: scheduler.Result{
			Skipped: true,
			Message: "ブロックリスト更新対象はありませんでした。",
		},
	})
	if ok {
		t.Fatalf("ok = true, want false")
	}
}

func TestBuildSchedulerNotificationShowsMissingLastRunGuidance(t *testing.T) {
	t.Parallel()

	notification, ok := buildSchedulerNotification(scheduler.Event{
		JobID: schedulerJobClassification,
		Kind:  scheduler.EventKindSkipped,
		Result: scheduler.Result{
			Skipped: true,
			Message: schedulerMessageMissingClassificationLastRun,
		},
	})
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if !strings.Contains(notification.Body, "last_run_at 未設定") {
		t.Fatalf("Body = %q, want contains last_run guidance", notification.Body)
	}
}

func TestBuildSchedulerNotificationAddsSettingsGuidanceOnCredentialError(t *testing.T) {
	t.Parallel()

	notification, ok := buildSchedulerNotification(scheduler.Event{
		JobID: schedulerJobKnownBlock,
		Kind:  scheduler.EventKindFailed,
		Err:   errors.New("Google token unauthorized"),
	})
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if !strings.Contains(notification.Body, "Settings 画面で Google 認証と Claude API キーを確認してください。") {
		t.Fatalf("Body = %q, want contains settings guidance", notification.Body)
	}
}

func TestEmitSchedulerNotificationSkipsWhenDisabled(t *testing.T) {
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

	if err := store.SetSetting(ctx, schedulerSettingNotificationsEnabled, "false"); err != nil {
		t.Fatalf("SetSetting returned error: %v", err)
	}

	called := 0
	app := &App{
		ctx:           ctx,
		dbStore:       store,
		databaseReady: true,
		eventsEmit: func(context.Context, string, ...interface{}) {
			called++
		},
	}

	app.emitSchedulerNotification(scheduler.Event{
		JobID: schedulerJobBlocklist,
		Kind:  scheduler.EventKindSucceeded,
		Result: scheduler.Result{
			Success: 1,
			Message: "ok",
		},
	})

	if called != 0 {
		t.Fatalf("eventsEmit called = %d, want 0", called)
	}
}

func TestEmitSchedulerNotificationEmitsWhenEnabled(t *testing.T) {
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

	called := 0
	gotEventName := ""
	var gotNotification types.SchedulerNotification

	app := &App{
		ctx:           ctx,
		dbStore:       store,
		databaseReady: true,
		eventsEmit: func(_ context.Context, eventName string, payload ...interface{}) {
			called++
			gotEventName = eventName
			if len(payload) == 1 {
				if notification, ok := payload[0].(types.SchedulerNotification); ok {
					gotNotification = notification
				}
			}
		},
	}

	app.emitSchedulerNotification(scheduler.Event{
		JobID: schedulerJobBlocklist,
		Kind:  scheduler.EventKindSucceeded,
		At:    time.Date(2026, time.March, 8, 15, 0, 0, 0, time.UTC),
		Result: scheduler.Result{
			Success: 3,
			Message: "ブロックリストを更新しました。",
		},
	})

	if called != 1 {
		t.Fatalf("eventsEmit called = %d, want 1", called)
	}
	if gotEventName != schedulerNotificationEventName {
		t.Fatalf("eventName = %q, want %q", gotEventName, schedulerNotificationEventName)
	}
	if gotNotification.Title == "" {
		t.Fatalf("notification.Title is empty")
	}
	if !strings.Contains(gotNotification.Body, "完了 3件") {
		t.Fatalf("notification.Body = %q, want contains summary", gotNotification.Body)
	}
}

func TestRunScheduledBlocklistRegistersSuggestions(t *testing.T) {
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

	for i := 0; i < 3; i++ {
		if err := store.RecordClassificationCorrection(ctx, types.ClassificationCorrection{
			MessageID:         "msg-sender",
			Sender:            "promo@example.com",
			OriginalCategory:  types.ClassificationCategoryNewsletter,
			CorrectedCategory: types.ClassificationCategoryJunk,
		}); err != nil {
			t.Fatalf("RecordClassificationCorrection returned error: %v", err)
		}
	}

	app := &App{
		dbStore:       store,
		databaseReady: true,
	}

	result, err := app.runScheduledBlocklist(ctx)
	if err != nil {
		t.Fatalf("runScheduledBlocklist returned error: %v", err)
	}
	if result.Success != 1 {
		t.Fatalf("result.Success = %d, want 1", result.Success)
	}
	if result.Processed != 1 {
		t.Fatalf("result.Processed = %d, want 1", result.Processed)
	}

	entries, err := store.ListBlocklistEntries(ctx)
	if err != nil {
		t.Fatalf("ListBlocklistEntries returned error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	if entries[0].Pattern != "promo@example.com" {
		t.Fatalf("entries[0].Pattern = %q, want %q", entries[0].Pattern, "promo@example.com")
	}
}

func TestRunScheduledClassificationSkipsWhenLastRunMissing(t *testing.T) {
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

	app := &App{
		ctx:           ctx,
		authClient:    auth.NewClient(auth.Config{}),
		secretManager: auth.NewSecretManager(auth.NewMemorySecretStore()),
		dbStore:       store,
		databaseReady: true,
	}

	result, err := app.runScheduledClassification(ctx)
	if err != nil {
		t.Fatalf("runScheduledClassification returned error: %v", err)
	}
	if !result.Skipped {
		t.Fatalf("result.Skipped = false, want true")
	}
	if result.Message != schedulerMessageMissingClassificationLastRun {
		t.Fatalf("result.Message = %q, want %q", result.Message, schedulerMessageMissingClassificationLastRun)
	}
}

func TestRunScheduledClassificationSkipsWhenCredentialsMissing(t *testing.T) {
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

	runAt := time.Date(2026, time.March, 8, 9, 0, 0, 0, time.UTC)
	if err := store.SetSetting(
		ctx,
		fmt.Sprintf(schedulerSettingLastRunTemplate, schedulerJobClassification),
		runAt.Format(time.RFC3339),
	); err != nil {
		t.Fatalf("SetSetting(job last_run_at) returned error: %v", err)
	}

	app := &App{
		ctx:           ctx,
		authClient:    auth.NewClient(auth.Config{}),
		secretManager: auth.NewSecretManager(auth.NewMemorySecretStore()),
		dbStore:       store,
		databaseReady: true,
	}

	result, err := app.runScheduledClassification(ctx)
	if err != nil {
		t.Fatalf("runScheduledClassification returned error: %v", err)
	}
	if !result.Skipped {
		t.Fatalf("result.Skipped = false, want true")
	}
	if !strings.Contains(result.Message, "Google トークンまたは Claude API キーが未設定") {
		t.Fatalf("result.Message = %q, want contains credential guidance", result.Message)
	}
}

func TestRunScheduledClassificationSkipsWhenNoNewMessages(t *testing.T) {
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

	secretStore := auth.NewMemorySecretStore()
	secretManager := auth.NewSecretManager(secretStore)
	if err := secretManager.SaveGoogleToken(ctx, auth.TokenSet{AccessToken: "access-token"}); err != nil {
		t.Fatalf("SaveGoogleToken returned error: %v", err)
	}
	if err := secretManager.SaveClaudeAPIKey(ctx, "claude-api-key"); err != nil {
		t.Fatalf("SaveClaudeAPIKey returned error: %v", err)
	}

	lastRunAt := time.Date(2026, time.March, 8, 9, 0, 0, 0, time.UTC)
	if err := store.SetSetting(
		ctx,
		fmt.Sprintf(schedulerSettingLastRunTemplate, schedulerJobClassification),
		lastRunAt.Format(time.RFC3339),
	); err != nil {
		t.Fatalf("SetSetting(job last_run_at) returned error: %v", err)
	}

	app := &App{
		ctx:           ctx,
		authClient:    auth.NewClient(auth.Config{}),
		secretManager: secretManager,
		gmailClient: gmail.NewClient(gmail.Options{
			BaseURL: "https://gmail.test",
			HTTPClient: &http.Client{
				Transport: appRoundTripFunc(func(r *http.Request) (*http.Response, error) {
					if r.URL.Path != "/gmail/v1/users/me/messages" {
						t.Fatalf("unexpected URL path: %s", r.URL.Path)
					}
					if got := r.URL.Query().Get("q"); got != "after:1772960400" {
						t.Fatalf("query q = %q, want %q", got, "after:1772960400")
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Header: http.Header{
							"Content-Type": []string{"application/json"},
						},
						Body: io.NopCloser(strings.NewReader(`{"messages":[]}`)),
					}, nil
				}),
			},
		}),
		dbStore:       store,
		databaseReady: true,
	}

	result, err := app.runScheduledClassification(ctx)
	if err != nil {
		t.Fatalf("runScheduledClassification returned error: %v", err)
	}
	if !result.Skipped {
		t.Fatalf("result.Skipped = false, want true")
	}
	if !strings.Contains(result.Message, "新着メールはありませんでした") {
		t.Fatalf("result.Message = %q, want contains no-new guidance", result.Message)
	}
}

func TestBuildSchedulerClassificationQuery(t *testing.T) {
	t.Parallel()

	runAt := time.Date(2026, time.March, 8, 9, 0, 0, 0, time.UTC)
	got := buildSchedulerClassificationQuery(runAt)
	if got != "after:1772960400" {
		t.Fatalf("query = %q, want %q", got, "after:1772960400")
	}

	if zero := buildSchedulerClassificationQuery(time.Time{}); zero != "" {
		t.Fatalf("zero query = %q, want empty", zero)
	}
}

func TestBuildSchedulerSafeRunDecisionsConvertsJunkAndCountsPending(t *testing.T) {
	t.Parallel()

	messages := []types.EmailSummary{
		{ID: "m1", ThreadID: "t1", From: "a@example.com", Subject: "one"},
		{ID: "m2", ThreadID: "t2", From: "b@example.com", Subject: "two"},
		{ID: "m3", ThreadID: "t3", From: "c@example.com", Subject: "three"},
	}
	results := []types.ClassificationResult{
		{
			MessageID:   "m1",
			Category:    types.ClassificationCategoryJunk,
			Confidence:  0.95,
			ReviewLevel: types.ClassificationReviewLevelAutoApply,
			Source:      types.ClassificationSourceClaude,
		},
		{
			MessageID:   "m2",
			Category:    types.ClassificationCategoryImportant,
			Confidence:  0.80,
			ReviewLevel: types.ClassificationReviewLevelReview,
			Source:      types.ClassificationSourceClaude,
		},
		{
			MessageID:   "m3",
			Category:    types.ClassificationCategoryArchive,
			Confidence:  0.91,
			ReviewLevel: types.ClassificationReviewLevelAutoApply,
			Source:      types.ClassificationSourceBlocklist,
		},
	}

	decisions, metadata, pendingApproval := buildSchedulerSafeRunDecisions(messages, results)
	if pendingApproval != 1 {
		t.Fatalf("pendingApproval = %d, want 1", pendingApproval)
	}
	if len(decisions) != 2 {
		t.Fatalf("len(decisions) = %d, want 2", len(decisions))
	}
	if decisions[0].Category != types.ClassificationCategoryArchive {
		t.Fatalf("decisions[0].Category = %q, want %q", decisions[0].Category, types.ClassificationCategoryArchive)
	}
	if decisions[1].Category != types.ClassificationCategoryArchive {
		t.Fatalf("decisions[1].Category = %q, want %q", decisions[1].Category, types.ClassificationCategoryArchive)
	}
	if len(metadata) != 2 {
		t.Fatalf("len(metadata) = %d, want 2", len(metadata))
	}
	if metadata[0].Category != types.ClassificationCategoryArchive {
		t.Fatalf("metadata[0].Category = %q, want %q", metadata[0].Category, types.ClassificationCategoryArchive)
	}
	if metadata[1].Source != types.ClassificationSourceBlocklist {
		t.Fatalf("metadata[1].Source = %q, want %q", metadata[1].Source, types.ClassificationSourceBlocklist)
	}
}

func TestRunScheduledClassificationPaginatesAllPages(t *testing.T) {
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

	secretStore := auth.NewMemorySecretStore()
	secretManager := auth.NewSecretManager(secretStore)
	if err := secretManager.SaveGoogleToken(ctx, auth.TokenSet{AccessToken: "access-token"}); err != nil {
		t.Fatalf("SaveGoogleToken returned error: %v", err)
	}
	if err := secretManager.SaveClaudeAPIKey(ctx, "claude-api-key"); err != nil {
		t.Fatalf("SaveClaudeAPIKey returned error: %v", err)
	}

	lastRunAt := time.Date(2026, time.March, 8, 9, 0, 0, 0, time.UTC)
	if err := store.SetSetting(
		ctx,
		fmt.Sprintf(schedulerSettingLastRunTemplate, schedulerJobClassification),
		lastRunAt.Format(time.RFC3339),
	); err != nil {
		t.Fatalf("SetSetting(job last_run_at) returned error: %v", err)
	}

	listCalls := 0
	claudeCalls := 0

	app := &App{
		ctx:           ctx,
		authClient:    auth.NewClient(auth.Config{}),
		secretManager: secretManager,
		gmailClient: gmail.NewClient(gmail.Options{
			BaseURL: "https://gmail.test",
			HTTPClient: &http.Client{
				Transport: appRoundTripFunc(func(r *http.Request) (*http.Response, error) {
					switch r.URL.Path {
					case "/gmail/v1/users/me/messages":
						listCalls++
						query := r.URL.Query()
						if got := query.Get("q"); got != "after:1772960400" {
							t.Fatalf("q = %q, want %q", got, "after:1772960400")
						}
						switch listCalls {
						case 1:
							if token := query.Get("pageToken"); token != "" {
								t.Fatalf("pageToken(1st) = %q, want empty", token)
							}
							return &http.Response{
								StatusCode: http.StatusOK,
								Header: http.Header{
									"Content-Type": []string{"application/json"},
								},
								Body: io.NopCloser(strings.NewReader(`{
									"messages":[{"id":"m1","threadId":"t1"}],
									"nextPageToken":"page-2"
								}`)),
							}, nil
						case 2:
							if token := query.Get("pageToken"); token != "page-2" {
								t.Fatalf("pageToken(2nd) = %q, want %q", token, "page-2")
							}
							return &http.Response{
								StatusCode: http.StatusOK,
								Header: http.Header{
									"Content-Type": []string{"application/json"},
								},
								Body: io.NopCloser(strings.NewReader(`{
									"messages":[{"id":"m2","threadId":"t2"}]
								}`)),
							}, nil
						default:
							t.Fatalf("unexpected list call count: %d", listCalls)
							return nil, nil
						}
					case "/gmail/v1/users/me/messages/m1":
						return &http.Response{
							StatusCode: http.StatusOK,
							Header: http.Header{
								"Content-Type": []string{"application/json"},
							},
							Body: io.NopCloser(strings.NewReader(`{
								"id":"m1",
								"threadId":"t1",
								"snippet":"one",
								"labelIds":["INBOX","UNREAD"],
								"payload":{"headers":[{"name":"From","value":"one@example.com"},{"name":"Subject","value":"one"}]}
							}`)),
						}, nil
					case "/gmail/v1/users/me/messages/m2":
						return &http.Response{
							StatusCode: http.StatusOK,
							Header: http.Header{
								"Content-Type": []string{"application/json"},
							},
							Body: io.NopCloser(strings.NewReader(`{
								"id":"m2",
								"threadId":"t2",
								"snippet":"two",
								"labelIds":["INBOX"],
								"payload":{"headers":[{"name":"From","value":"two@example.com"},{"name":"Subject","value":"two"}]}
							}`)),
						}, nil
					default:
						t.Fatalf("unexpected Gmail URL: %s", r.URL.String())
						return nil, nil
					}
				}),
			},
		}),
		claudeClient: claude.NewClient(claude.Options{
			BaseURL: "https://claude.test",
			HTTPClient: &http.Client{
				Transport: appRoundTripFunc(func(r *http.Request) (*http.Response, error) {
					if r.URL.Path != "/v1/messages" {
						t.Fatalf("unexpected Claude URL: %s", r.URL.String())
					}
					claudeCalls++
					var body string
					switch claudeCalls {
					case 1:
						body = `{"content":[{"type":"text","text":"{\"results\":[{\"id\":\"m1\",\"category\":\"important\",\"confidence\":0.8,\"reason\":\"review\"}]}"}]}`
					case 2:
						body = `{"content":[{"type":"text","text":"{\"results\":[{\"id\":\"m2\",\"category\":\"newsletter\",\"confidence\":0.8,\"reason\":\"review\"}]}"}]}`
					default:
						t.Fatalf("unexpected Claude call count: %d", claudeCalls)
						return nil, nil
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Header: http.Header{
							"Content-Type": []string{"application/json"},
						},
						Body: io.NopCloser(strings.NewReader(body)),
					}, nil
				}),
			},
		}),
		dbStore:       store,
		databaseReady: true,
	}

	result, err := app.runScheduledClassification(ctx)
	if err != nil {
		t.Fatalf("runScheduledClassification returned error: %v", err)
	}
	if result.Processed != 2 {
		t.Fatalf("result.Processed = %d, want 2", result.Processed)
	}
	if result.PendingApproval != 2 {
		t.Fatalf("result.PendingApproval = %d, want 2", result.PendingApproval)
	}
	if listCalls != 2 {
		t.Fatalf("listCalls = %d, want 2", listCalls)
	}
	if claudeCalls != 2 {
		t.Fatalf("claudeCalls = %d, want 2", claudeCalls)
	}
}

func TestRunScheduledClassificationResumesFromCheckpoint(t *testing.T) {
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

	secretStore := auth.NewMemorySecretStore()
	secretManager := auth.NewSecretManager(secretStore)
	if err := secretManager.SaveGoogleToken(ctx, auth.TokenSet{AccessToken: "access-token"}); err != nil {
		t.Fatalf("SaveGoogleToken returned error: %v", err)
	}
	if err := secretManager.SaveClaudeAPIKey(ctx, "claude-api-key"); err != nil {
		t.Fatalf("SaveClaudeAPIKey returned error: %v", err)
	}

	lastRunAt := time.Date(2026, time.March, 8, 9, 0, 0, 0, time.UTC)
	if err := store.SetSetting(
		ctx,
		fmt.Sprintf(schedulerSettingLastRunTemplate, schedulerJobClassification),
		lastRunAt.Format(time.RFC3339),
	); err != nil {
		t.Fatalf("SetSetting(job last_run_at) returned error: %v", err)
	}

	checkpoint := classificationCheckpoint{
		RunType:          schedulerCheckpointRunTypeScheduled,
		Mode:             schedulerCheckpointModeSafeRun,
		LastRunAt:        lastRunAt.Format(time.RFC3339),
		Query:            "after:1772960400",
		LabelIDs:         []string{"INBOX"},
		CompletedBatches: 1,
		Processed:        1,
		Success:          0,
		Failed:           0,
		PendingApproval:  1,
		UpdatedAt:        time.Now().UTC().Format(time.RFC3339),
	}
	if err := storeClassificationCheckpoint(ctx, store, checkpoint); err != nil {
		t.Fatalf("storeClassificationCheckpoint returned error: %v", err)
	}

	listCalls := 0
	claudeCalls := 0

	app := &App{
		ctx:           ctx,
		authClient:    auth.NewClient(auth.Config{}),
		secretManager: secretManager,
		gmailClient: gmail.NewClient(gmail.Options{
			BaseURL: "https://gmail.test",
			HTTPClient: &http.Client{
				Transport: appRoundTripFunc(func(r *http.Request) (*http.Response, error) {
					switch r.URL.Path {
					case "/gmail/v1/users/me/messages":
						listCalls++
						query := r.URL.Query()
						if got := query.Get("q"); got != "after:1772960400" {
							t.Fatalf("q = %q, want %q", got, "after:1772960400")
						}
						switch listCalls {
						case 1:
							return &http.Response{
								StatusCode: http.StatusOK,
								Header: http.Header{
									"Content-Type": []string{"application/json"},
								},
								Body: io.NopCloser(strings.NewReader(`{
									"messages":[{"id":"m1","threadId":"t1"}],
									"nextPageToken":"page-2"
								}`)),
							}, nil
						case 2:
							if token := query.Get("pageToken"); token != "page-2" {
								t.Fatalf("pageToken(2nd) = %q, want %q", token, "page-2")
							}
							return &http.Response{
								StatusCode: http.StatusOK,
								Header: http.Header{
									"Content-Type": []string{"application/json"},
								},
								Body: io.NopCloser(strings.NewReader(`{
									"messages":[{"id":"m2","threadId":"t2"}]
								}`)),
							}, nil
						default:
							t.Fatalf("unexpected list call count: %d", listCalls)
							return nil, nil
						}
					case "/gmail/v1/users/me/messages/m1":
						return &http.Response{
							StatusCode: http.StatusOK,
							Header: http.Header{
								"Content-Type": []string{"application/json"},
							},
							Body: io.NopCloser(strings.NewReader(`{
								"id":"m1",
								"threadId":"t1",
								"snippet":"one",
								"labelIds":["INBOX","UNREAD"],
								"payload":{"headers":[{"name":"From","value":"one@example.com"},{"name":"Subject","value":"one"}]}
							}`)),
						}, nil
					case "/gmail/v1/users/me/messages/m2":
						return &http.Response{
							StatusCode: http.StatusOK,
							Header: http.Header{
								"Content-Type": []string{"application/json"},
							},
							Body: io.NopCloser(strings.NewReader(`{
								"id":"m2",
								"threadId":"t2",
								"snippet":"two",
								"labelIds":["INBOX"],
								"payload":{"headers":[{"name":"From","value":"two@example.com"},{"name":"Subject","value":"two"}]}
							}`)),
						}, nil
					default:
						t.Fatalf("unexpected Gmail URL: %s", r.URL.String())
						return nil, nil
					}
				}),
			},
		}),
		claudeClient: claude.NewClient(claude.Options{
			BaseURL: "https://claude.test",
			HTTPClient: &http.Client{
				Transport: appRoundTripFunc(func(r *http.Request) (*http.Response, error) {
					claudeCalls++
					if r.URL.Path != "/v1/messages" {
						t.Fatalf("unexpected Claude URL: %s", r.URL.String())
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Header: http.Header{
							"Content-Type": []string{"application/json"},
						},
						Body: io.NopCloser(strings.NewReader(`{
							"content":[{"type":"text","text":"{\"results\":[{\"id\":\"m2\",\"category\":\"important\",\"confidence\":0.8,\"reason\":\"review\"}]}"}]
						}`)),
					}, nil
				}),
			},
		}),
		dbStore:       store,
		databaseReady: true,
	}

	result, err := app.runScheduledClassification(ctx)
	if err != nil {
		t.Fatalf("runScheduledClassification returned error: %v", err)
	}
	if result.Processed != 2 {
		t.Fatalf("result.Processed = %d, want 2", result.Processed)
	}
	if result.PendingApproval != 2 {
		t.Fatalf("result.PendingApproval = %d, want 2", result.PendingApproval)
	}
	if listCalls != 2 {
		t.Fatalf("listCalls = %d, want 2", listCalls)
	}
	if claudeCalls != 1 {
		t.Fatalf("claudeCalls = %d, want 1", claudeCalls)
	}

	stored, ok, err := loadClassificationCheckpoint(ctx, store)
	if err != nil {
		t.Fatalf("loadClassificationCheckpoint returned error: %v", err)
	}
	if !ok {
		t.Fatalf("checkpoint was not stored")
	}
	if stored.CompletedBatches != 2 {
		t.Fatalf("stored.CompletedBatches = %d, want 2", stored.CompletedBatches)
	}
	if stored.Processed != 2 {
		t.Fatalf("stored.Processed = %d, want 2", stored.Processed)
	}
}

func TestRunScheduledClassificationKeepsCheckpointOnBatchFailure(t *testing.T) {
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

	secretStore := auth.NewMemorySecretStore()
	secretManager := auth.NewSecretManager(secretStore)
	if err := secretManager.SaveGoogleToken(ctx, auth.TokenSet{AccessToken: "access-token"}); err != nil {
		t.Fatalf("SaveGoogleToken returned error: %v", err)
	}
	if err := secretManager.SaveClaudeAPIKey(ctx, "claude-api-key"); err != nil {
		t.Fatalf("SaveClaudeAPIKey returned error: %v", err)
	}

	lastRunAt := time.Date(2026, time.March, 8, 9, 0, 0, 0, time.UTC)
	if err := store.SetSetting(
		ctx,
		fmt.Sprintf(schedulerSettingLastRunTemplate, schedulerJobClassification),
		lastRunAt.Format(time.RFC3339),
	); err != nil {
		t.Fatalf("SetSetting(job last_run_at) returned error: %v", err)
	}

	listCalls := 0
	claudeCalls := 0

	app := &App{
		ctx:           ctx,
		authClient:    auth.NewClient(auth.Config{}),
		secretManager: secretManager,
		gmailClient: gmail.NewClient(gmail.Options{
			BaseURL: "https://gmail.test",
			HTTPClient: &http.Client{
				Transport: appRoundTripFunc(func(r *http.Request) (*http.Response, error) {
					switch r.URL.Path {
					case "/gmail/v1/users/me/messages":
						listCalls++
						query := r.URL.Query()
						if got := query.Get("q"); got != "after:1772960400" {
							t.Fatalf("q = %q, want %q", got, "after:1772960400")
						}
						switch listCalls {
						case 1:
							return &http.Response{
								StatusCode: http.StatusOK,
								Header: http.Header{
									"Content-Type": []string{"application/json"},
								},
								Body: io.NopCloser(strings.NewReader(`{
									"messages":[{"id":"m1","threadId":"t1"}],
									"nextPageToken":"page-2"
								}`)),
							}, nil
						case 2:
							return &http.Response{
								StatusCode: http.StatusOK,
								Header: http.Header{
									"Content-Type": []string{"application/json"},
								},
								Body: io.NopCloser(strings.NewReader(`{
									"messages":[{"id":"m2","threadId":"t2"}]
								}`)),
							}, nil
						default:
							t.Fatalf("unexpected list call count: %d", listCalls)
							return nil, nil
						}
					case "/gmail/v1/users/me/messages/m1":
						return &http.Response{
							StatusCode: http.StatusOK,
							Header: http.Header{
								"Content-Type": []string{"application/json"},
							},
							Body: io.NopCloser(strings.NewReader(`{
								"id":"m1",
								"threadId":"t1",
								"snippet":"one",
								"labelIds":["INBOX","UNREAD"],
								"payload":{"headers":[{"name":"From","value":"one@example.com"},{"name":"Subject","value":"one"}]}
							}`)),
						}, nil
					case "/gmail/v1/users/me/messages/m2":
						return &http.Response{
							StatusCode: http.StatusOK,
							Header: http.Header{
								"Content-Type": []string{"application/json"},
							},
							Body: io.NopCloser(strings.NewReader(`{
								"id":"m2",
								"threadId":"t2",
								"snippet":"two",
								"labelIds":["INBOX"],
								"payload":{"headers":[{"name":"From","value":"two@example.com"},{"name":"Subject","value":"two"}]}
							}`)),
						}, nil
					default:
						t.Fatalf("unexpected Gmail URL: %s", r.URL.String())
						return nil, nil
					}
				}),
			},
		}),
		claudeClient: claude.NewClient(claude.Options{
			BaseURL: "https://claude.test",
			HTTPClient: &http.Client{
				Transport: appRoundTripFunc(func(r *http.Request) (*http.Response, error) {
					claudeCalls++
					if r.URL.Path != "/v1/messages" {
						t.Fatalf("unexpected Claude URL: %s", r.URL.String())
					}
					if claudeCalls == 1 {
						return &http.Response{
							StatusCode: http.StatusOK,
							Header: http.Header{
								"Content-Type": []string{"application/json"},
							},
							Body: io.NopCloser(strings.NewReader(`{
								"content":[{"type":"text","text":"{\"results\":[{\"id\":\"m1\",\"category\":\"important\",\"confidence\":0.8,\"reason\":\"review\"}]}"}]
							}`)),
						}, nil
					}
					return &http.Response{
						StatusCode: http.StatusInternalServerError,
						Header: http.Header{
							"Content-Type": []string{"application/json"},
						},
						Body: io.NopCloser(strings.NewReader(`{"error":{"type":"api_error","message":"temporary"}}`)),
					}, nil
				}),
			},
		}),
		dbStore:       store,
		databaseReady: true,
	}

	result, err := app.runScheduledClassification(ctx)
	if err == nil {
		t.Fatalf("runScheduledClassification error = nil, want non-nil")
	}
	if result.Processed != 1 {
		t.Fatalf("result.Processed = %d, want 1", result.Processed)
	}
	if result.PendingApproval != 1 {
		t.Fatalf("result.PendingApproval = %d, want 1", result.PendingApproval)
	}
	if !scheduler.IsRetryable(err) {
		t.Fatalf("error should be retryable: %v", err)
	}

	stored, ok, err := loadClassificationCheckpoint(ctx, store)
	if err != nil {
		t.Fatalf("loadClassificationCheckpoint returned error: %v", err)
	}
	if !ok {
		t.Fatalf("checkpoint was not stored")
	}
	if stored.CompletedBatches != 1 {
		t.Fatalf("stored.CompletedBatches = %d, want 1", stored.CompletedBatches)
	}
	if stored.Processed != 1 {
		t.Fatalf("stored.Processed = %d, want 1", stored.Processed)
	}
	if stored.LastStopReason != "" {
		t.Fatalf("stored.LastStopReason = %q, want empty", stored.LastStopReason)
	}
}

func TestRunScheduledClassificationStoresCheckpointOnFirstBatchFailure(t *testing.T) {
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

	secretStore := auth.NewMemorySecretStore()
	secretManager := auth.NewSecretManager(secretStore)
	if err := secretManager.SaveGoogleToken(ctx, auth.TokenSet{AccessToken: "access-token"}); err != nil {
		t.Fatalf("SaveGoogleToken returned error: %v", err)
	}
	if err := secretManager.SaveClaudeAPIKey(ctx, "claude-api-key"); err != nil {
		t.Fatalf("SaveClaudeAPIKey returned error: %v", err)
	}

	lastRunAt := time.Date(2026, time.March, 8, 9, 0, 0, 0, time.UTC)
	if err := store.SetSetting(
		ctx,
		fmt.Sprintf(schedulerSettingLastRunTemplate, schedulerJobClassification),
		lastRunAt.Format(time.RFC3339),
	); err != nil {
		t.Fatalf("SetSetting(job last_run_at) returned error: %v", err)
	}

	app := &App{
		ctx:           ctx,
		authClient:    auth.NewClient(auth.Config{}),
		secretManager: secretManager,
		gmailClient: gmail.NewClient(gmail.Options{
			BaseURL: "https://gmail.test",
			HTTPClient: &http.Client{
				Transport: appRoundTripFunc(func(r *http.Request) (*http.Response, error) {
					switch r.URL.Path {
					case "/gmail/v1/users/me/messages":
						return &http.Response{
							StatusCode: http.StatusOK,
							Header: http.Header{
								"Content-Type": []string{"application/json"},
							},
							Body: io.NopCloser(strings.NewReader(`{
								"messages":[{"id":"m1","threadId":"t1"}]
							}`)),
						}, nil
					case "/gmail/v1/users/me/messages/m1":
						return &http.Response{
							StatusCode: http.StatusOK,
							Header: http.Header{
								"Content-Type": []string{"application/json"},
							},
							Body: io.NopCloser(strings.NewReader(`{
								"id":"m1",
								"threadId":"t1",
								"snippet":"one",
								"labelIds":["INBOX","UNREAD"],
								"payload":{"headers":[{"name":"From","value":"one@example.com"},{"name":"Subject","value":"one"}]}
							}`)),
						}, nil
					default:
						t.Fatalf("unexpected Gmail URL: %s", r.URL.String())
						return nil, nil
					}
				}),
			},
		}),
		claudeClient: claude.NewClient(claude.Options{
			BaseURL: "https://claude.test",
			HTTPClient: &http.Client{
				Transport: appRoundTripFunc(func(*http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusInternalServerError,
						Header: http.Header{
							"Content-Type": []string{"application/json"},
						},
						Body: io.NopCloser(strings.NewReader(`{"error":{"type":"api_error","message":"temporary"}}`)),
					}, nil
				}),
			},
		}),
		dbStore:       store,
		databaseReady: true,
	}

	result, err := app.runScheduledClassification(ctx)
	if err == nil {
		t.Fatalf("runScheduledClassification error = nil, want non-nil")
	}
	if result.Processed != 0 {
		t.Fatalf("result.Processed = %d, want 0", result.Processed)
	}
	if !scheduler.IsRetryable(err) {
		t.Fatalf("error should be retryable: %v", err)
	}

	stored, ok, err := loadClassificationCheckpoint(ctx, store)
	if err != nil {
		t.Fatalf("loadClassificationCheckpoint returned error: %v", err)
	}
	if !ok {
		t.Fatalf("checkpoint was not stored")
	}
	if stored.CompletedBatches != 0 {
		t.Fatalf("stored.CompletedBatches = %d, want 0", stored.CompletedBatches)
	}
	if stored.Processed != 0 {
		t.Fatalf("stored.Processed = %d, want 0", stored.Processed)
	}
}

func TestRunScheduledClassificationDoesNotMutateAggregatedWhenCheckpointSaveFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := db.Open(ctx, db.OpenOptions{
		Path: filepath.Join(t.TempDir(), "mairu.db"),
	})
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}
	closed := false
	t.Cleanup(func() {
		if closed {
			return
		}
		if err := store.Close(); err != nil {
			t.Fatalf("store.Close returned error: %v", err)
		}
	})

	secretStore := auth.NewMemorySecretStore()
	secretManager := auth.NewSecretManager(secretStore)
	if err := secretManager.SaveGoogleToken(ctx, auth.TokenSet{AccessToken: "access-token"}); err != nil {
		t.Fatalf("SaveGoogleToken returned error: %v", err)
	}
	if err := secretManager.SaveClaudeAPIKey(ctx, "claude-api-key"); err != nil {
		t.Fatalf("SaveClaudeAPIKey returned error: %v", err)
	}

	lastRunAt := time.Date(2026, time.March, 8, 9, 0, 0, 0, time.UTC)
	if err := store.SetSetting(
		ctx,
		fmt.Sprintf(schedulerSettingLastRunTemplate, schedulerJobClassification),
		lastRunAt.Format(time.RFC3339),
	); err != nil {
		t.Fatalf("SetSetting(job last_run_at) returned error: %v", err)
	}

	app := &App{
		ctx:           ctx,
		authClient:    auth.NewClient(auth.Config{}),
		secretManager: secretManager,
		gmailClient: gmail.NewClient(gmail.Options{
			BaseURL: "https://gmail.test",
			HTTPClient: &http.Client{
				Transport: appRoundTripFunc(func(r *http.Request) (*http.Response, error) {
					switch r.URL.Path {
					case "/gmail/v1/users/me/messages":
						return &http.Response{
							StatusCode: http.StatusOK,
							Header: http.Header{
								"Content-Type": []string{"application/json"},
							},
							Body: io.NopCloser(strings.NewReader(`{
								"messages":[{"id":"m1","threadId":"t1"}]
							}`)),
						}, nil
					case "/gmail/v1/users/me/messages/m1":
						return &http.Response{
							StatusCode: http.StatusOK,
							Header: http.Header{
								"Content-Type": []string{"application/json"},
							},
							Body: io.NopCloser(strings.NewReader(`{
								"id":"m1",
								"threadId":"t1",
								"snippet":"one",
								"labelIds":["INBOX","UNREAD"],
								"payload":{"headers":[{"name":"From","value":"one@example.com"},{"name":"Subject","value":"one"}]}
							}`)),
						}, nil
					default:
						t.Fatalf("unexpected Gmail URL: %s", r.URL.String())
						return nil, nil
					}
				}),
			},
		}),
		claudeClient: claude.NewClient(claude.Options{
			BaseURL: "https://claude.test",
			HTTPClient: &http.Client{
				Transport: appRoundTripFunc(func(*http.Request) (*http.Response, error) {
					if err := store.Close(); err != nil {
						t.Fatalf("store.Close returned error: %v", err)
					}
					closed = true
					return &http.Response{
						StatusCode: http.StatusOK,
						Header: http.Header{
							"Content-Type": []string{"application/json"},
						},
						Body: io.NopCloser(strings.NewReader(`{
							"content":[{"type":"text","text":"{\"results\":[{\"id\":\"m1\",\"category\":\"important\",\"confidence\":0.8,\"reason\":\"review\"}]}"}]
						}`)),
					}, nil
				}),
			},
		}),
		dbStore:       store,
		databaseReady: true,
	}

	result, err := app.runScheduledClassification(ctx)
	if err == nil {
		t.Fatalf("runScheduledClassification error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "分類 checkpoint を保存できませんでした") {
		t.Fatalf("error = %v, want checkpoint save error", err)
	}
	if result.Processed != 0 {
		t.Fatalf("result.Processed = %d, want 0", result.Processed)
	}
	if result.PendingApproval != 0 {
		t.Fatalf("result.PendingApproval = %d, want 0", result.PendingApproval)
	}
}

func TestMaybeMarkSchedulerRetryableRecognizesParenthesizedStatus(t *testing.T) {
	t.Parallel()

	err := maybeMarkSchedulerRetryable(errors.New("Gmail API 取得に失敗しました (503): temporary"))
	if !scheduler.IsRetryable(err) {
		t.Fatalf("error should be retryable: %v", err)
	}
}

func TestLogSchedulerEventStoresLastRunAtOnClassificationSucceeded(t *testing.T) {
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

	app := &App{
		ctx:           ctx,
		authClient:    auth.NewClient(auth.Config{}),
		secretManager: auth.NewSecretManager(auth.NewMemorySecretStore()),
		dbStore:       store,
		databaseReady: true,
	}

	checkpoint := classificationCheckpoint{
		RunType:          schedulerCheckpointRunTypeScheduled,
		Mode:             schedulerCheckpointModeSafeRun,
		LastRunAt:        time.Date(2026, time.March, 8, 9, 0, 0, 0, time.UTC).Format(time.RFC3339),
		Query:            "after:1772960400",
		LabelIDs:         []string{"INBOX"},
		CompletedBatches: 2,
		Processed:        100,
		UpdatedAt:        time.Now().UTC().Format(time.RFC3339),
	}
	if err := storeClassificationCheckpoint(ctx, store, checkpoint); err != nil {
		t.Fatalf("storeClassificationCheckpoint returned error: %v", err)
	}

	runAt := time.Date(2026, time.March, 8, 12, 34, 56, 0, time.UTC)
	app.logSchedulerEvent(scheduler.Event{
		JobID:   schedulerJobClassification,
		Kind:    scheduler.EventKindSucceeded,
		Attempt: 1,
		At:      runAt,
	})

	got, ok, err := store.GetSetting(ctx, schedulerSettingLastRunAt)
	if err != nil {
		t.Fatalf("GetSetting(lastRunAt) returned error: %v", err)
	}
	if !ok {
		t.Fatalf("schedulerSettingLastRunAt was not stored")
	}
	if got != runAt.Format(time.RFC3339) {
		t.Fatalf("lastRunAt = %q, want %q", got, runAt.Format(time.RFC3339))
	}

	jobKey := fmt.Sprintf(schedulerSettingLastRunTemplate, schedulerJobClassification)
	gotJob, ok, err := store.GetSetting(ctx, jobKey)
	if err != nil {
		t.Fatalf("GetSetting(jobLastRunAt) returned error: %v", err)
	}
	if !ok {
		t.Fatalf("job last_run_at was not stored")
	}
	if gotJob != runAt.Format(time.RFC3339) {
		t.Fatalf("job last_run_at = %q, want %q", gotJob, runAt.Format(time.RFC3339))
	}

	checkpointValue, ok, err := store.GetSetting(ctx, schedulerSettingClassificationCheckpoint)
	if err != nil {
		t.Fatalf("GetSetting(checkpoint) returned error: %v", err)
	}
	if !ok {
		t.Fatalf("classification checkpoint setting was not stored")
	}
	if strings.TrimSpace(checkpointValue) != "" {
		t.Fatalf("classification checkpoint = %q, want empty", checkpointValue)
	}
}

func TestLogSchedulerEventStoresCheckpointStopReasonOnFailure(t *testing.T) {
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

	checkpoint := classificationCheckpoint{
		RunType:          schedulerCheckpointRunTypeScheduled,
		Mode:             schedulerCheckpointModeSafeRun,
		LastRunAt:        time.Date(2026, time.March, 8, 9, 0, 0, 0, time.UTC).Format(time.RFC3339),
		Query:            "after:1772960400",
		LabelIDs:         []string{"INBOX"},
		CompletedBatches: 1,
		Processed:        50,
		UpdatedAt:        time.Now().UTC().Format(time.RFC3339),
	}
	if err := storeClassificationCheckpoint(ctx, store, checkpoint); err != nil {
		t.Fatalf("storeClassificationCheckpoint returned error: %v", err)
	}

	app := &App{
		ctx:           ctx,
		authClient:    auth.NewClient(auth.Config{}),
		secretManager: auth.NewSecretManager(auth.NewMemorySecretStore()),
		dbStore:       store,
		databaseReady: true,
	}

	app.logSchedulerEvent(scheduler.Event{
		JobID:      schedulerJobClassification,
		Kind:       scheduler.EventKindFailed,
		Attempt:    1,
		MaxRetries: 3,
		Err:        context.Canceled,
	})

	stored, ok, err := loadClassificationCheckpoint(ctx, store)
	if err != nil {
		t.Fatalf("loadClassificationCheckpoint returned error: %v", err)
	}
	if !ok {
		t.Fatalf("checkpoint was not stored")
	}
	if stored.LastStopReason != "canceled (キャンセル)" {
		t.Fatalf("stored.LastStopReason = %q, want %q", stored.LastStopReason, "canceled (キャンセル)")
	}
	if stored.LastError != context.Canceled.Error() {
		t.Fatalf("stored.LastError = %q, want %q", stored.LastError, context.Canceled.Error())
	}
}

func TestLogSchedulerEventDoesNotStoreLastRunAtOnStarted(t *testing.T) {
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

	app := &App{
		ctx:           ctx,
		authClient:    auth.NewClient(auth.Config{}),
		secretManager: auth.NewSecretManager(auth.NewMemorySecretStore()),
		dbStore:       store,
		databaseReady: true,
	}

	app.logSchedulerEvent(scheduler.Event{
		JobID:   schedulerJobClassification,
		Kind:    scheduler.EventKindStarted,
		Attempt: 1,
		At:      time.Date(2026, time.March, 8, 12, 34, 56, 0, time.UTC),
	})

	_, ok, err := store.GetSetting(ctx, schedulerSettingLastRunAt)
	if err != nil {
		t.Fatalf("GetSetting(lastRunAt) returned error: %v", err)
	}
	if ok {
		t.Fatalf("schedulerSettingLastRunAt was stored on started event")
	}
}

func TestStartSchedulerAllowsManualTrigger(t *testing.T) {
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

	app := &App{
		ctx:           ctx,
		authClient:    auth.NewClient(auth.Config{}),
		secretManager: auth.NewSecretManager(auth.NewMemorySecretStore()),
		dbStore:       store,
		databaseReady: true,
	}
	t.Cleanup(app.stopScheduler)

	called := make(chan struct{}, 1)
	app.runScheduledClassificationJob = func(context.Context) (scheduler.Result, error) {
		select {
		case called <- struct{}{}:
		default:
		}
		return scheduler.Result{Skipped: true, Message: "test"}, nil
	}

	if err := app.startScheduler(); err != nil {
		t.Fatalf("startScheduler returned error: %v", err)
	}

	if app.schedulerSvc == nil {
		t.Fatalf("schedulerSvc = nil, want non-nil")
	}
	if !app.schedulerSvc.Trigger(schedulerJobClassification) {
		t.Fatalf("Trigger(classification) = false, want true")
	}

	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatalf("manual trigger timeout")
	}
}

func TestStartSchedulerSkipsUnimplementedJobs(t *testing.T) {
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

	app := &App{
		ctx:           ctx,
		authClient:    auth.NewClient(auth.Config{}),
		secretManager: auth.NewSecretManager(auth.NewMemorySecretStore()),
		dbStore:       store,
		databaseReady: true,
	}
	t.Cleanup(app.stopScheduler)

	if err := app.startScheduler(); err != nil {
		t.Fatalf("startScheduler returned error: %v", err)
	}

	if app.schedulerSvc == nil {
		t.Fatalf("schedulerSvc = nil, want non-nil")
	}
	if !app.schedulerSvc.Trigger(schedulerJobClassification) {
		t.Fatalf("Trigger(classification) = false, want true")
	}
	if app.schedulerSvc.Trigger(schedulerJobKnownBlock) {
		t.Fatalf("Trigger(known_block) = true, want false")
	}
	if !app.schedulerSvc.Trigger(schedulerJobBlocklist) {
		t.Fatalf("Trigger(blocklist) = false, want true")
	}
}

func writeTestExecutableScript(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "gws-test.sh")
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
	return path
}

type appRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn appRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func storeClassificationCheckpoint(
	ctx context.Context,
	store *db.Store,
	checkpoint classificationCheckpoint,
) error {
	data, err := json.Marshal(checkpoint)
	if err != nil {
		return err
	}
	return store.SetSetting(ctx, schedulerSettingClassificationCheckpoint, string(data))
}

func loadClassificationCheckpoint(
	ctx context.Context,
	store *db.Store,
) (classificationCheckpoint, bool, error) {
	value, ok, err := store.GetSetting(ctx, schedulerSettingClassificationCheckpoint)
	if err != nil {
		return classificationCheckpoint{}, false, err
	}
	if !ok || strings.TrimSpace(value) == "" {
		return classificationCheckpoint{}, false, nil
	}

	var checkpoint classificationCheckpoint
	if err := json.Unmarshal([]byte(value), &checkpoint); err != nil {
		return classificationCheckpoint{}, false, err
	}
	return checkpoint, true, nil
}
