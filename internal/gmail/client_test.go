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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}
