package gmail

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCheckConnection(t *testing.T) {
	t.Parallel()

	client := NewClient(Options{
		BaseURL: "https://gmail.test",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodGet {
					t.Fatalf("unexpected method: got %s, want %s", r.Method, http.MethodGet)
				}
				if r.URL.String() != "https://gmail.test/gmail/v1/users/me/profile" {
					t.Fatalf("unexpected URL: got %s", r.URL.String())
				}
				if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
					t.Fatalf("Authorization mismatch: got %q", got)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: io.NopCloser(strings.NewReader(`{
						"emailAddress":"user@example.com",
						"messagesTotal":42,
						"threadsTotal":13,
						"historyId":"987654"
					}`)),
				}, nil
			}),
		},
	})

	profile, err := client.CheckConnection(context.Background(), "access-token")
	if err != nil {
		t.Fatalf("CheckConnection returned error: %v", err)
	}

	if profile.EmailAddress != "user@example.com" {
		t.Fatalf("EmailAddress mismatch: got %q", profile.EmailAddress)
	}
	if profile.MessagesTotal != 42 {
		t.Fatalf("MessagesTotal mismatch: got %d", profile.MessagesTotal)
	}
	if profile.ThreadsTotal != 13 {
		t.Fatalf("ThreadsTotal mismatch: got %d", profile.ThreadsTotal)
	}
	if profile.HistoryID != "987654" {
		t.Fatalf("HistoryID mismatch: got %q", profile.HistoryID)
	}
}

func TestCheckConnectionReturnsAPIError(t *testing.T) {
	t.Parallel()

	client := NewClient(Options{
		BaseURL: "https://gmail.test",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusUnauthorized,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: io.NopCloser(strings.NewReader(`{
						"error":{
							"code":401,
							"message":"Request had invalid authentication credentials."
						}
					}`)),
				}, nil
			}),
		},
	})

	_, err := client.CheckConnection(context.Background(), "access-token")
	if err == nil {
		t.Fatalf("CheckConnection returned nil error, want error")
	}
	if !strings.Contains(err.Error(), "invalid authentication credentials") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetchMessages(t *testing.T) {
	t.Parallel()

	client := NewClient(Options{
		BaseURL: "https://gmail.test",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
					t.Fatalf("Authorization mismatch: got %q", got)
				}
				if r.Method != http.MethodGet {
					t.Fatalf("unexpected method: got %s, want %s", r.Method, http.MethodGet)
				}

				switch {
				case r.URL.Path == "/gmail/v1/users/me/messages":
					query := r.URL.Query()
					if got := query.Get("maxResults"); got != "2" {
						t.Fatalf("maxResults = %q, want 2", got)
					}
					if got := query.Get("q"); got != "after:1700000000" {
						t.Fatalf("q = %q, want %q", got, "after:1700000000")
					}
					labelIDs := query["labelIds"]
					if len(labelIDs) != 1 || labelIDs[0] != "INBOX" {
						t.Fatalf("labelIds = %v, want [INBOX]", labelIDs)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Header: http.Header{
							"Content-Type": []string{"application/json"},
						},
						Body: io.NopCloser(strings.NewReader(`{
							"messages":[
								{"id":"m1","threadId":"t1"},
								{"id":"m2","threadId":"t2"}
							],
							"nextPageToken":"next-page-token"
						}`)),
					}, nil
				case r.URL.Path == "/gmail/v1/users/me/messages/m1":
					return &http.Response{
						StatusCode: http.StatusOK,
						Header: http.Header{
							"Content-Type": []string{"application/json"},
						},
						Body: io.NopCloser(strings.NewReader(`{
							"id":"m1",
							"threadId":"t1",
							"snippet":"first message",
							"labelIds":["INBOX","UNREAD"],
							"payload":{
								"headers":[
									{"name":"From","value":"alpha@example.com"},
									{"name":"Subject","value":"subject-1"}
								]
							}
						}`)),
					}, nil
				case r.URL.Path == "/gmail/v1/users/me/messages/m2":
					return &http.Response{
						StatusCode: http.StatusOK,
						Header: http.Header{
							"Content-Type": []string{"application/json"},
						},
						Body: io.NopCloser(strings.NewReader(`{
							"id":"m2",
							"threadId":"t2",
							"snippet":"second message",
							"labelIds":["INBOX"],
							"payload":{
								"headers":[
									{"name":"From","value":"beta@example.com"},
									{"name":"Subject","value":"subject-2"}
								]
							}
						}`)),
					}, nil
				default:
					t.Fatalf("unexpected URL: %s", r.URL.String())
					return nil, nil
				}
			}),
		},
	})

	result, err := client.FetchMessages(context.Background(), "access-token", FetchRequest{
		MaxResults: 2,
		LabelIDs:   []string{"INBOX"},
		Query:      "after:1700000000",
	})
	if err != nil {
		t.Fatalf("FetchMessages returned error: %v", err)
	}
	if len(result.Messages) != 2 {
		t.Fatalf("len(Messages) = %d, want 2", len(result.Messages))
	}
	if result.NextPageToken != "next-page-token" {
		t.Fatalf("NextPageToken = %q, want %q", result.NextPageToken, "next-page-token")
	}
	if result.Messages[0].ID != "m1" {
		t.Fatalf("Messages[0].ID = %q, want %q", result.Messages[0].ID, "m1")
	}
	if !result.Messages[0].Unread {
		t.Fatalf("Messages[0].Unread = false, want true")
	}
	if result.Messages[1].Unread {
		t.Fatalf("Messages[1].Unread = true, want false")
	}
}

func TestFetchMessagesSkipsMessageDetailFailures(t *testing.T) {
	t.Parallel()

	client := NewClient(Options{
		BaseURL: "https://gmail.test",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/gmail/v1/users/me/messages" {
					return &http.Response{
						StatusCode: http.StatusOK,
						Header: http.Header{
							"Content-Type": []string{"application/json"},
						},
						Body: io.NopCloser(strings.NewReader(`{
							"messages":[
								{"id":"missing","threadId":"t1"},
								{"id":"ok","threadId":"t2"}
							]
						}`)),
					}, nil
				}

				switch r.URL.Path {
				case "/gmail/v1/users/me/messages/missing":
					return &http.Response{
						StatusCode: http.StatusNotFound,
						Header: http.Header{
							"Content-Type": []string{"application/json"},
						},
						Body: io.NopCloser(strings.NewReader(`{
							"error":{"code":404,"message":"Not found"}
						}`)),
					}, nil
				case "/gmail/v1/users/me/messages/ok":
					return &http.Response{
						StatusCode: http.StatusOK,
						Header: http.Header{
							"Content-Type": []string{"application/json"},
						},
						Body: io.NopCloser(strings.NewReader(`{
							"id":"ok",
							"threadId":"t2",
							"snippet":"usable",
							"labelIds":["INBOX"],
							"payload":{"headers":[{"name":"From","value":"ok@example.com"},{"name":"Subject","value":"ok"}]}
						}`)),
					}, nil
				default:
					t.Fatalf("unexpected URL: %s", r.URL.String())
					return nil, nil
				}
			}),
		},
	})

	result, err := client.FetchMessages(context.Background(), "access-token", FetchRequest{
		MaxResults: 2,
		LabelIDs:   []string{"INBOX"},
	})
	if err != nil {
		t.Fatalf("FetchMessages returned error: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("len(Messages) = %d, want 1", len(result.Messages))
	}
	if result.Messages[0].ID != "ok" {
		t.Fatalf("Messages[0].ID = %q, want %q", result.Messages[0].ID, "ok")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}
