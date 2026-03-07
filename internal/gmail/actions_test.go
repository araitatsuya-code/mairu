package gmail

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"mairu/internal/types"
)

func TestExecuteActionsCreatesLabelsAndAppliesOperations(t *testing.T) {
	t.Parallel()

	step := 0
	client := NewClient(Options{
		BaseURL: "https://gmail.test",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch step {
				case 0:
					if r.Method != http.MethodGet {
						t.Fatalf("step0 method: got %s, want %s", r.Method, http.MethodGet)
					}
					if r.URL.String() != "https://gmail.test/gmail/v1/users/me/labels" {
						t.Fatalf("step0 url: got %s", r.URL.String())
					}
					step++
					return jsonResponse(http.StatusOK, `{"labels":[{"id":"INBOX","name":"INBOX","type":"system"}]}`), nil
				case 1:
					if r.Method != http.MethodPost {
						t.Fatalf("step1 method: got %s, want %s", r.Method, http.MethodPost)
					}
					if r.URL.String() != "https://gmail.test/gmail/v1/users/me/labels" {
						t.Fatalf("step1 url: got %s", r.URL.String())
					}
					var request createLabelRequest
					mustDecodeJSONBody(t, r, &request)
					if request.Name != mairuLabelArchive {
						t.Fatalf("step1 label: got %q, want %q", request.Name, mairuLabelArchive)
					}
					step++
					return jsonResponse(http.StatusOK, `{"id":"LabelArchive","name":"Mairu/Archive","type":"user"}`), nil
				case 2:
					if r.Method != http.MethodPost {
						t.Fatalf("step2 method: got %s, want %s", r.Method, http.MethodPost)
					}
					if r.URL.String() != "https://gmail.test/gmail/v1/users/me/labels" {
						t.Fatalf("step2 url: got %s", r.URL.String())
					}
					var request createLabelRequest
					mustDecodeJSONBody(t, r, &request)
					if request.Name != mairuLabelImportant {
						t.Fatalf("step2 label: got %q, want %q", request.Name, mairuLabelImportant)
					}
					step++
					return jsonResponse(http.StatusOK, `{"id":"LabelImportant","name":"Mairu/Important","type":"user"}`), nil
				case 3:
					if r.Method != http.MethodPost {
						t.Fatalf("step3 method: got %s, want %s", r.Method, http.MethodPost)
					}
					if r.URL.String() != "https://gmail.test/gmail/v1/users/me/messages/msg-1/modify" {
						t.Fatalf("step3 url: got %s", r.URL.String())
					}
					var request messageModifyRequest
					mustDecodeJSONBody(t, r, &request)
					if len(request.AddLabelIDs) != 1 || request.AddLabelIDs[0] != "LabelImportant" {
						t.Fatalf("step3 addLabelIds: got %v", request.AddLabelIDs)
					}
					if len(request.RemoveLabelIDs) != 0 {
						t.Fatalf("step3 removeLabelIds: got %v", request.RemoveLabelIDs)
					}
					step++
					return jsonResponse(http.StatusOK, `{}`), nil
				case 4:
					if r.Method != http.MethodPost {
						t.Fatalf("step4 method: got %s, want %s", r.Method, http.MethodPost)
					}
					if r.URL.String() != "https://gmail.test/gmail/v1/users/me/messages/msg-2/modify" {
						t.Fatalf("step4 url: got %s", r.URL.String())
					}
					var request messageModifyRequest
					mustDecodeJSONBody(t, r, &request)
					if len(request.AddLabelIDs) != 1 || request.AddLabelIDs[0] != "LabelArchive" {
						t.Fatalf("step4 addLabelIds: got %v", request.AddLabelIDs)
					}
					if len(request.RemoveLabelIDs) != 1 || request.RemoveLabelIDs[0] != systemLabelInbox {
						t.Fatalf("step4 removeLabelIds: got %v", request.RemoveLabelIDs)
					}
					step++
					return jsonResponse(http.StatusOK, `{}`), nil
				case 5:
					if r.Method != http.MethodPost {
						t.Fatalf("step5 method: got %s, want %s", r.Method, http.MethodPost)
					}
					if r.URL.String() != "https://gmail.test/gmail/v1/users/me/messages/msg-3/trash" {
						t.Fatalf("step5 url: got %s", r.URL.String())
					}
					if r.Body != nil {
						body, err := io.ReadAll(r.Body)
						if err != nil {
							t.Fatalf("step5 read body: %v", err)
						}
						if strings.TrimSpace(string(body)) != "" {
							t.Fatalf("step5 body: got %q, want empty", string(body))
						}
					}
					step++
					return &http.Response{
						StatusCode: http.StatusNoContent,
						Header:     http.Header{},
						Body:       io.NopCloser(strings.NewReader("")),
					}, nil
				default:
					t.Fatalf("unexpected step: %d (%s %s)", step, r.Method, r.URL.String())
					return nil, nil
				}
			}),
		},
	})

	result, err := client.ExecuteActions(context.Background(), "access-token", []types.GmailActionDecision{
		{
			MessageID:   "msg-1",
			Category:    types.ClassificationCategoryImportant,
			ReviewLevel: types.ClassificationReviewLevelReview,
		},
		{
			MessageID:   "msg-2",
			Category:    types.ClassificationCategoryArchive,
			ReviewLevel: types.ClassificationReviewLevelAutoApply,
		},
		{
			MessageID:   "msg-3",
			Category:    types.ClassificationCategoryJunk,
			ReviewLevel: types.ClassificationReviewLevelReview,
		},
	})
	if err != nil {
		t.Fatalf("ExecuteActions returned error: %v", err)
	}

	if !result.Success {
		t.Fatalf("Success = false, want true")
	}
	if result.ProcessedCount != 3 {
		t.Fatalf("ProcessedCount = %d, want 3", result.ProcessedCount)
	}
	if result.SuccessCount != 3 {
		t.Fatalf("SuccessCount = %d, want 3", result.SuccessCount)
	}
	if result.FailureCount != 0 {
		t.Fatalf("FailureCount = %d, want 0", result.FailureCount)
	}
	if result.DeletedCount != 1 {
		t.Fatalf("DeletedCount = %d, want 1", result.DeletedCount)
	}
	if result.ArchivedCount != 1 {
		t.Fatalf("ArchivedCount = %d, want 1", result.ArchivedCount)
	}
	if result.MarkedReadCount != 0 {
		t.Fatalf("MarkedReadCount = %d, want 0", result.MarkedReadCount)
	}
	if result.LabeledCount != 2 {
		t.Fatalf("LabeledCount = %d, want 2", result.LabeledCount)
	}
	if got := strings.Join(result.CreatedLabels, ","); got != "Mairu/Archive,Mairu/Important" {
		t.Fatalf("CreatedLabels = %v", result.CreatedLabels)
	}
	if step != 6 {
		t.Fatalf("step count = %d, want 6", step)
	}
}

func TestExecuteActionsKeepsProcessingOnPartialFailure(t *testing.T) {
	t.Parallel()

	step := 0
	client := NewClient(Options{
		BaseURL: "https://gmail.test",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch step {
				case 0:
					step++
					return jsonResponse(http.StatusOK, `{"labels":[{"id":"LabelImportant","name":"Mairu/Important","type":"user"}]}`), nil
				case 1:
					step++
					return jsonResponse(http.StatusBadRequest, `{"error":{"code":400,"message":"invalid modify request"}}`), nil
				case 2:
					if r.URL.String() != "https://gmail.test/gmail/v1/users/me/messages/msg-2/trash" {
						t.Fatalf("step2 url: got %s", r.URL.String())
					}
					step++
					return &http.Response{
						StatusCode: http.StatusNoContent,
						Header:     http.Header{},
						Body:       io.NopCloser(strings.NewReader("")),
					}, nil
				default:
					t.Fatalf("unexpected step: %d", step)
					return nil, nil
				}
			}),
		},
	})

	result, err := client.ExecuteActions(context.Background(), "access-token", []types.GmailActionDecision{
		{
			MessageID:   "msg-1",
			Category:    types.ClassificationCategoryImportant,
			ReviewLevel: types.ClassificationReviewLevelReview,
		},
		{
			MessageID:   "msg-2",
			Category:    types.ClassificationCategoryJunk,
			ReviewLevel: types.ClassificationReviewLevelReview,
		},
	})
	if err != nil {
		t.Fatalf("ExecuteActions returned error: %v", err)
	}

	if result.Success {
		t.Fatalf("Success = true, want false")
	}
	if result.ProcessedCount != 2 {
		t.Fatalf("ProcessedCount = %d, want 2", result.ProcessedCount)
	}
	if result.SuccessCount != 1 {
		t.Fatalf("SuccessCount = %d, want 1", result.SuccessCount)
	}
	if result.FailureCount != 1 {
		t.Fatalf("FailureCount = %d, want 1", result.FailureCount)
	}
	if result.DeletedCount != 1 {
		t.Fatalf("DeletedCount = %d, want 1", result.DeletedCount)
	}
	if len(result.Failures) != 1 {
		t.Fatalf("Failures length = %d, want 1", len(result.Failures))
	}
	if result.Failures[0].MessageID != "msg-1" {
		t.Fatalf("Failure MessageID = %q, want %q", result.Failures[0].MessageID, "msg-1")
	}
	if result.Failures[0].Action != types.ActionKindLabel {
		t.Fatalf("Failure Action = %q, want %q", result.Failures[0].Action, types.ActionKindLabel)
	}
	if !strings.Contains(result.Failures[0].Error, "invalid modify request") {
		t.Fatalf("Failure Error = %q, want contains invalid modify request", result.Failures[0].Error)
	}
	if step != 3 {
		t.Fatalf("step count = %d, want 3", step)
	}
}

func TestBuildActionPlansAddsNeedsReviewLabel(t *testing.T) {
	t.Parallel()

	plans, requiredLabels, err := buildActionPlans([]types.GmailActionDecision{
		{
			MessageID:   "msg-1",
			Category:    types.ClassificationCategoryUnreadPriority,
			ReviewLevel: types.ClassificationReviewLevelReviewWithReason,
		},
	})
	if err != nil {
		t.Fatalf("buildActionPlans returned error: %v", err)
	}
	if len(plans) != 1 {
		t.Fatalf("plans length = %d, want 1", len(plans))
	}
	if len(plans[0].addSystemLabelIDs) != 1 || plans[0].addSystemLabelIDs[0] != systemLabelUnread {
		t.Fatalf("addSystemLabelIDs = %v, want [%s]", plans[0].addSystemLabelIDs, systemLabelUnread)
	}
	if len(requiredLabels) != 2 {
		t.Fatalf("requiredLabels length = %d, want 2", len(requiredLabels))
	}
	if got := strings.Join(requiredLabels, ","); got != "Mairu/Needs Review,Mairu/Unread Priority" {
		t.Fatalf("requiredLabels = %v", requiredLabels)
	}
}

func TestBuildActionPlansSkipsNeedsReviewLabelForDelete(t *testing.T) {
	t.Parallel()

	plans, requiredLabels, err := buildActionPlans([]types.GmailActionDecision{
		{
			MessageID:   "msg-1",
			Category:    types.ClassificationCategoryJunk,
			ReviewLevel: types.ClassificationReviewLevelHold,
		},
	})
	if err != nil {
		t.Fatalf("buildActionPlans returned error: %v", err)
	}
	if len(plans) != 1 {
		t.Fatalf("plans length = %d, want 1", len(plans))
	}
	if !plans[0].delete {
		t.Fatalf("delete = false, want true")
	}
	if len(plans[0].addLabelNames) != 0 {
		t.Fatalf("addLabelNames = %v, want empty", plans[0].addLabelNames)
	}
	if len(requiredLabels) != 0 {
		t.Fatalf("requiredLabels = %v, want empty", requiredLabels)
	}
}

func TestExecuteActionsLabelCreateRaceFallsBackToRelist(t *testing.T) {
	t.Parallel()

	step := 0
	client := NewClient(Options{
		BaseURL: "https://gmail.test",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch step {
				case 0:
					if r.URL.String() != "https://gmail.test/gmail/v1/users/me/labels" {
						t.Fatalf("step0 url: got %s", r.URL.String())
					}
					step++
					return jsonResponse(http.StatusOK, `{"labels":[]}`), nil
				case 1:
					if r.URL.String() != "https://gmail.test/gmail/v1/users/me/labels" {
						t.Fatalf("step1 url: got %s", r.URL.String())
					}
					step++
					return jsonResponse(http.StatusConflict, `{"error":{"code":409,"message":"label already exists"}}`), nil
				case 2:
					if r.URL.String() != "https://gmail.test/gmail/v1/users/me/labels" {
						t.Fatalf("step2 url: got %s", r.URL.String())
					}
					step++
					return jsonResponse(http.StatusOK, `{"labels":[{"id":"LabelImportant","name":"Mairu/Important","type":"user"}]}`), nil
				case 3:
					if r.URL.String() != "https://gmail.test/gmail/v1/users/me/messages/msg-1/modify" {
						t.Fatalf("step3 url: got %s", r.URL.String())
					}
					var request messageModifyRequest
					mustDecodeJSONBody(t, r, &request)
					if len(request.AddLabelIDs) != 1 || request.AddLabelIDs[0] != "LabelImportant" {
						t.Fatalf("step3 addLabelIds: got %v", request.AddLabelIDs)
					}
					step++
					return jsonResponse(http.StatusOK, `{}`), nil
				default:
					t.Fatalf("unexpected step: %d", step)
					return nil, nil
				}
			}),
		},
	})

	result, err := client.ExecuteActions(context.Background(), "access-token", []types.GmailActionDecision{
		{
			MessageID:   "msg-1",
			Category:    types.ClassificationCategoryImportant,
			ReviewLevel: types.ClassificationReviewLevelReview,
		},
	})
	if err != nil {
		t.Fatalf("ExecuteActions returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("Success = false, want true")
	}
	if len(result.CreatedLabels) != 0 {
		t.Fatalf("CreatedLabels = %v, want empty", result.CreatedLabels)
	}
	if step != 4 {
		t.Fatalf("step count = %d, want 4", step)
	}
}

func mustDecodeJSONBody(t *testing.T, request *http.Request, destination any) {
	t.Helper()
	defer request.Body.Close()

	if err := json.NewDecoder(request.Body).Decode(destination); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
}

func jsonResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}
}
