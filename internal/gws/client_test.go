package gws

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDetectReportsNotInstalled(t *testing.T) {
	t.Parallel()

	client := NewClient(Options{BinaryPath: "definitely-not-found-gws-binary"})
	result := client.Detect()

	if result.Available {
		t.Fatalf("Available = true, want false")
	}
	if result.ErrorKind != ErrorKindNotInstalled {
		t.Fatalf("ErrorKind = %q, want %q", result.ErrorKind, ErrorKindNotInstalled)
	}
}

func TestDiagnoseReturnsVersion(t *testing.T) {
	t.Parallel()

	binaryPath := writeExecutableScript(t, `#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "gws 0.9.0"
  exit 0
fi

echo "unexpected args" >&2
exit 3
`)
	client := NewClient(Options{BinaryPath: binaryPath})

	result := client.Diagnose(context.Background())
	if !result.Success {
		t.Fatalf("Success = false, want true (message=%q, output=%q)", result.Message, result.Output)
	}
	if result.Version != "gws 0.9.0" {
		t.Fatalf("Version = %q, want %q", result.Version, "gws 0.9.0")
	}
	if result.ErrorKind != ErrorKindNone {
		t.Fatalf("ErrorKind = %q, want %q", result.ErrorKind, ErrorKindNone)
	}
}

func TestRunGmailListDryRunIncludesDryRunAndParams(t *testing.T) {
	t.Parallel()

	binaryPath := writeExecutableScript(t, `#!/bin/sh
printf "%s\n" "$@"
`)
	client := NewClient(Options{BinaryPath: binaryPath})

	result := client.RunGmailListDryRun(context.Background(), GmailDryRunRequest{
		Query:      "label:inbox is:unread",
		MaxResults: 15,
	})
	if !result.Success {
		t.Fatalf("Success = false, want true (message=%q, output=%q)", result.Message, result.Output)
	}

	if !strings.Contains(result.Output, "--dry-run") {
		t.Fatalf("Output does not include --dry-run: %q", result.Output)
	}
	if !strings.Contains(result.Output, `"userId":"me"`) {
		t.Fatalf("Output does not include userId=me: %q", result.Output)
	}
	if !strings.Contains(result.Output, `"maxResults":15`) {
		t.Fatalf("Output does not include maxResults=15: %q", result.Output)
	}
	if !strings.Contains(result.Output, `"q":"label:inbox is:unread"`) {
		t.Fatalf("Output does not include q query: %q", result.Output)
	}
}

func TestRunGmailListDryRunMapsExitCodeToErrorKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		exitCode int
		wantKind ErrorKind
	}{
		{name: "auth", exitCode: 2, wantKind: ErrorKindAuth},
		{name: "invalid", exitCode: 3, wantKind: ErrorKindInvalidCommand},
		{name: "execution", exitCode: 5, wantKind: ErrorKindExecution},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			binaryPath := writeExecutableScript(t, "#!/bin/sh\necho 'boom' >&2\nexit "+strconv.Itoa(tt.exitCode)+"\n")
			client := NewClient(Options{BinaryPath: binaryPath})

			result := client.RunGmailListDryRun(context.Background(), GmailDryRunRequest{})
			if result.Success {
				t.Fatalf("Success = true, want false")
			}
			if result.ErrorKind != tt.wantKind {
				t.Fatalf("ErrorKind = %q, want %q", result.ErrorKind, tt.wantKind)
			}
		})
	}
}

func TestRunGmailListDryRunTimeout(t *testing.T) {
	t.Parallel()

	binaryPath := writeExecutableScript(t, `#!/bin/sh
sleep 2
`)
	client := NewClient(Options{BinaryPath: binaryPath})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result := client.RunGmailListDryRun(ctx, GmailDryRunRequest{})
	if result.Success {
		t.Fatalf("Success = true, want false")
	}
	if result.ErrorKind != ErrorKindTimeout {
		t.Fatalf("ErrorKind = %q, want %q", result.ErrorKind, ErrorKindTimeout)
	}
}

func writeExecutableScript(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "gws-test.sh")
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
	return path
}
