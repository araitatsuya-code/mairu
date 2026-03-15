package claude

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"mairu/internal/types"
)

func TestClassify(t *testing.T) {
	t.Parallel()

	client := NewClient(Options{
		BaseURL:      "https://claude.test",
		DefaultModel: "claude-test-model",
		HTTPClient: &http.Client{
			Transport: claudeRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodPost {
					t.Fatalf("unexpected method: got %s, want %s", r.Method, http.MethodPost)
				}
				if r.URL.String() != "https://claude.test/v1/messages" {
					t.Fatalf("unexpected URL: got %s", r.URL.String())
				}
				if got := r.Header.Get("x-api-key"); got != "claude-secret" {
					t.Fatalf("x-api-key mismatch: got %q", got)
				}
				if got := r.Header.Get("anthropic-version"); got != defaultAPIVersion {
					t.Fatalf("anthropic-version mismatch: got %q", got)
				}

				payload, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("ReadAll returned error: %v", err)
				}

				var body struct {
					Model    string `json:"model"`
					Messages []struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					} `json:"messages"`
				}
				if err := json.Unmarshal(payload, &body); err != nil {
					t.Fatalf("json.Unmarshal returned error: %v", err)
				}
				if body.Model != "claude-test-model" {
					t.Fatalf("model = %q, want %q", body.Model, "claude-test-model")
				}
				if len(body.Messages) != 1 {
					t.Fatalf("messages length = %d, want 1", len(body.Messages))
				}
				if !strings.Contains(body.Messages[0].Content, `"id":"msg-1"`) {
					t.Fatalf("prompt does not contain msg-1: %q", body.Messages[0].Content)
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
								"text":"[{\"id\":\"msg-2\",\"category\":\"newsletter\",\"confidence\":0.88,\"reason\":\"定期配信です\"},{\"id\":\"msg-1\",\"category\":\"important\",\"confidence\":0.95,\"reason\":\"顧客対応が必要です\"}]"
							}
						]
					}`)),
				}, nil
			}),
		},
	})

	result, err := client.Classify(context.Background(), " claude-secret ", types.ClassificationRequest{
		Messages: []types.EmailSummary{
			{ID: "msg-1", From: "boss@example.com", Subject: "至急確認", Snippet: "確認をお願いします", Unread: true},
			{ID: "msg-2", From: "news@example.com", Subject: "今週のニュース", Snippet: "最新情報です", Unread: false},
		},
	})
	if err != nil {
		t.Fatalf("Classify returned error: %v", err)
	}

	if result.Model != "claude-test-model" {
		t.Fatalf("Model = %q, want %q", result.Model, "claude-test-model")
	}
	if len(result.Results) != 2 {
		t.Fatalf("Results length = %d, want 2", len(result.Results))
	}
	if result.Results[0].MessageID != "msg-1" {
		t.Fatalf("Results[0].MessageID = %q, want %q", result.Results[0].MessageID, "msg-1")
	}
	if result.Results[0].ReviewLevel != types.ClassificationReviewLevelAutoApply {
		t.Fatalf("Results[0].ReviewLevel = %q, want %q", result.Results[0].ReviewLevel, types.ClassificationReviewLevelAutoApply)
	}
	if result.Results[1].ReviewLevel != types.ClassificationReviewLevelReview {
		t.Fatalf("Results[1].ReviewLevel = %q, want %q", result.Results[1].ReviewLevel, types.ClassificationReviewLevelReview)
	}
}

func TestClassifyRejectsTooManyMessages(t *testing.T) {
	t.Parallel()

	client := NewClient(Options{MaxBatchSize: 2})

	_, err := client.Classify(context.Background(), "claude-secret", types.ClassificationRequest{
		Messages: []types.EmailSummary{
			{ID: "msg-1"},
			{ID: "msg-2"},
			{ID: "msg-3"},
		},
	})
	if err == nil {
		t.Fatalf("Classify returned nil error, want error")
	}
	if !strings.Contains(err.Error(), "最大 2 件") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClassifyReturnsAPIError(t *testing.T) {
	t.Parallel()

	client := NewClient(Options{
		BaseURL: "https://claude.test",
		HTTPClient: &http.Client{
			Transport: claudeRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusTooManyRequests,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: io.NopCloser(strings.NewReader(`{
						"error":{
							"type":"rate_limit_error",
							"message":"rate limit exceeded"
						}
					}`)),
				}, nil
			}),
		},
	})

	_, err := client.Classify(context.Background(), "claude-secret", types.ClassificationRequest{
		Messages: []types.EmailSummary{
			{ID: "msg-1"},
		},
	})
	if err == nil {
		t.Fatalf("Classify returned nil error, want error")
	}
	if !strings.Contains(err.Error(), "rate limit exceeded") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClassifyRejectsMissingResult(t *testing.T) {
	t.Parallel()

	client := NewClient(Options{
		BaseURL: "https://claude.test",
		HTTPClient: &http.Client{
			Transport: claudeRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: io.NopCloser(strings.NewReader(`{
						"content":[
							{
								"type":"text",
								"text":"[{\"id\":\"msg-1\",\"category\":\"important\",\"confidence\":0.95,\"reason\":\"要対応\"}]"
							}
						]
					}`)),
				}, nil
			}),
		},
	})

	_, err := client.Classify(context.Background(), "claude-secret", types.ClassificationRequest{
		Messages: []types.EmailSummary{
			{ID: "msg-1"},
			{ID: "msg-2"},
		},
	})
	if err == nil {
		t.Fatalf("Classify returned nil error, want error")
	}
	if !strings.Contains(err.Error(), "応答件数が一致しません") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClassifyParsesJSONEmbeddedInText(t *testing.T) {
	t.Parallel()

	client := NewClient(Options{
		BaseURL:      "https://claude.test",
		DefaultModel: "claude-test-model",
		HTTPClient: &http.Client{
			Transport: claudeRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: io.NopCloser(strings.NewReader(`{
							"content":[
								{
									"type":"text",
									"text":"以下が分類結果です。\n結果: [{\"id\":\"msg-1\",\"category\":\"important\",\"confidence\":0.91,\"reason\":\"要返信\"}]\n確認してください。"
								}
							]
						}`)),
				}, nil
			}),
		},
	})

	result, err := client.Classify(context.Background(), "claude-secret", types.ClassificationRequest{
		Messages: []types.EmailSummary{
			{ID: "msg-1", Subject: "subject"},
		},
	})
	if err != nil {
		t.Fatalf("Classify returned error: %v", err)
	}
	if len(result.Results) != 1 {
		t.Fatalf("Results length = %d, want 1", len(result.Results))
	}
	if result.Results[0].MessageID != "msg-1" {
		t.Fatalf("MessageID = %q, want %q", result.Results[0].MessageID, "msg-1")
	}
}

func TestClassifySkipsMismatchedCandidateAndUsesLaterValidCandidate(t *testing.T) {
	t.Parallel()

	client := NewClient(Options{
		BaseURL:      "https://claude.test",
		DefaultModel: "claude-test-model",
		HTTPClient: &http.Client{
			Transport: claudeRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: io.NopCloser(strings.NewReader(`{
						"content":[
							{
								"type":"text",
								"text":"補助情報: []\n最終結果: [{\"id\":\"msg-1\",\"category\":\"important\",\"confidence\":0.91,\"reason\":\"要返信\"}]"
							}
						]
					}`)),
				}, nil
			}),
		},
	})

	result, err := client.Classify(context.Background(), "claude-secret", types.ClassificationRequest{
		Messages: []types.EmailSummary{
			{ID: "msg-1", Subject: "subject"},
		},
	})
	if err != nil {
		t.Fatalf("Classify returned error: %v", err)
	}
	if len(result.Results) != 1 {
		t.Fatalf("Results length = %d, want 1", len(result.Results))
	}
	if result.Results[0].MessageID != "msg-1" {
		t.Fatalf("MessageID = %q, want %q", result.Results[0].MessageID, "msg-1")
	}
}

type claudeRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn claudeRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}
