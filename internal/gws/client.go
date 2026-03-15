package gws

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultBinaryName  = "gws"
	defaultMaxResults  = 20
	minimumMaxResults  = 1
	maximumMaxResults  = 100
	defaultGmailUserID = "me"
)

// ErrorKind は gws 実行失敗時の分類を表す。
type ErrorKind string

const (
	ErrorKindNone           ErrorKind = "none"
	ErrorKindNotInstalled   ErrorKind = "not_installed"
	ErrorKindAuth           ErrorKind = "auth"
	ErrorKindInvalidCommand ErrorKind = "invalid_command"
	ErrorKindTimeout        ErrorKind = "timeout"
	ErrorKindExecution      ErrorKind = "execution"
)

// Options は gws クライアント生成時の設定を表す。
type Options struct {
	BinaryPath string
	LookPath   func(string) (string, error)
}

// Detection は gws バイナリ検出結果を表す。
type Detection struct {
	Available  bool
	BinaryPath string
	ErrorKind  ErrorKind
	Message    string
}

// DiagnoseResult は `gws --version` 実行結果を表す。
type DiagnoseResult struct {
	Success    bool
	Available  bool
	BinaryPath string
	Version    string
	Command    string
	Output     string
	ErrorKind  ErrorKind
	Message    string
}

// GmailDryRunRequest は Gmail read-only dry-run 実行入力を表す。
type GmailDryRunRequest struct {
	Query      string
	MaxResults int
}

// GmailDryRunResult は Gmail read-only dry-run 実行結果を表す。
type GmailDryRunResult struct {
	Success    bool
	BinaryPath string
	Command    string
	Output     string
	ErrorKind  ErrorKind
	Message    string
}

// Client は gws コマンド実行ラッパー。
type Client struct {
	binaryPath string
	lookPath   func(string) (string, error)
}

// NewClient は gws 実行クライアントを構築する。
func NewClient(options Options) *Client {
	lookPath := options.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}

	return &Client{
		binaryPath: strings.TrimSpace(options.BinaryPath),
		lookPath:   lookPath,
	}
}

// Detect は gws の導入状態を確認する。
func (c *Client) Detect() Detection {
	resolvedPath, err := c.resolveBinaryPath()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return Detection{
				Available: false,
				ErrorKind: ErrorKindNotInstalled,
				Message:   "gws が見つかりません。PATH へ追加するかインストールしてください。",
			}
		}

		return Detection{
			Available: false,
			ErrorKind: ErrorKindExecution,
			Message:   fmt.Sprintf("gws の実行パス確認に失敗しました: %v", err),
		}
	}

	return Detection{
		Available:  true,
		BinaryPath: resolvedPath,
		ErrorKind:  ErrorKindNone,
		Message:    fmt.Sprintf("gws を利用できます (%s)", resolvedPath),
	}
}

// Diagnose は `gws --version` を実行し接続診断を返す。
func (c *Client) Diagnose(ctx context.Context) DiagnoseResult {
	detection := c.Detect()
	if !detection.Available {
		return DiagnoseResult{
			Success:    false,
			Available:  false,
			BinaryPath: "",
			ErrorKind:  detection.ErrorKind,
			Message:    detection.Message,
		}
	}

	args := []string{"--version"}
	command := formatCommand(detection.BinaryPath, args)
	output, errKind, message, runErr := c.run(ctx, detection.BinaryPath, args)
	if runErr != nil {
		return DiagnoseResult{
			Success:    false,
			Available:  true,
			BinaryPath: detection.BinaryPath,
			Command:    command,
			Output:     output,
			ErrorKind:  errKind,
			Message:    message,
		}
	}

	version := firstLine(output)
	if version == "" {
		version = "unknown"
	}

	return DiagnoseResult{
		Success:    true,
		Available:  true,
		BinaryPath: detection.BinaryPath,
		Version:    version,
		Command:    command,
		Output:     output,
		ErrorKind:  ErrorKindNone,
		Message:    "gws 診断に成功しました。",
	}
}

// RunGmailListDryRun は Gmail read-only list コマンドを `--dry-run` で実行する。
func (c *Client) RunGmailListDryRun(ctx context.Context, request GmailDryRunRequest) GmailDryRunResult {
	detection := c.Detect()
	if !detection.Available {
		return GmailDryRunResult{
			Success:    false,
			BinaryPath: "",
			ErrorKind:  detection.ErrorKind,
			Message:    detection.Message,
		}
	}

	maxResults := normalizeMaxResults(request.MaxResults)
	params := map[string]any{
		"userId":     defaultGmailUserID,
		"maxResults": maxResults,
	}
	if query := strings.TrimSpace(request.Query); query != "" {
		params["q"] = query
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return GmailDryRunResult{
			Success:    false,
			BinaryPath: detection.BinaryPath,
			ErrorKind:  ErrorKindExecution,
			Message:    fmt.Sprintf("gws dry-run のパラメータ生成に失敗しました: %v", err),
		}
	}

	args := []string{
		"gmail", "users", "messages", "list",
		"--params", string(paramsJSON),
		"--dry-run",
	}
	command := formatCommand(detection.BinaryPath, args)
	output, errKind, message, runErr := c.run(ctx, detection.BinaryPath, args)
	if runErr != nil {
		return GmailDryRunResult{
			Success:    false,
			BinaryPath: detection.BinaryPath,
			Command:    command,
			Output:     output,
			ErrorKind:  errKind,
			Message:    message,
		}
	}

	return GmailDryRunResult{
		Success:    true,
		BinaryPath: detection.BinaryPath,
		Command:    command,
		Output:     output,
		ErrorKind:  ErrorKindNone,
		Message:    "gws Gmail dry-run の実行に成功しました。",
	}
}

func (c *Client) resolveBinaryPath() (string, error) {
	configured := strings.TrimSpace(c.binaryPath)
	if configured == "" {
		configured = defaultBinaryName
	}

	if strings.ContainsAny(configured, `/\`) || filepath.IsAbs(configured) {
		if _, err := os.Stat(configured); err != nil {
			return "", err
		}
		return configured, nil
	}

	return c.lookPath(configured)
}

func (c *Client) run(
	ctx context.Context,
	binaryPath string,
	args []string,
) (output string, errKind ErrorKind, message string, runErr error) {
	command := exec.CommandContext(ctx, binaryPath, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	err := command.Run()
	combined := buildCombinedOutput(stdout.String(), stderr.String())
	if err == nil {
		return combined, ErrorKindNone, "", nil
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(ctx.Err(), context.Canceled) {
		return combined, ErrorKindTimeout, "gws 実行がタイムアウトしました。", err
	}

	if errors.Is(err, exec.ErrNotFound) {
		return combined, ErrorKindNotInstalled, "gws が見つかりません。", err
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitCode := exitErr.ExitCode()
		errKind := mapExitCode(exitCode)
		detail := firstLine(strings.TrimSpace(stderr.String()))
		if detail == "" {
			detail = firstLine(strings.TrimSpace(stdout.String()))
		}
		if detail == "" {
			detail = fmt.Sprintf("exit code=%d", exitCode)
		}

		return combined, errKind, fmt.Sprintf("gws コマンドが失敗しました (%s): %s", errKind, detail), err
	}

	return combined, ErrorKindExecution, fmt.Sprintf("gws コマンドの実行に失敗しました: %v", err), err
}

func normalizeMaxResults(value int) int {
	switch {
	case value < minimumMaxResults:
		return defaultMaxResults
	case value > maximumMaxResults:
		return maximumMaxResults
	default:
		return value
	}
}

func mapExitCode(code int) ErrorKind {
	switch code {
	case 2:
		return ErrorKindAuth
	case 3:
		return ErrorKindInvalidCommand
	default:
		return ErrorKindExecution
	}
}

func buildCombinedOutput(stdout string, stderr string) string {
	trimmedStdout := strings.TrimSpace(stdout)
	trimmedStderr := strings.TrimSpace(stderr)
	switch {
	case trimmedStdout != "" && trimmedStderr != "":
		return trimmedStdout + "\n" + trimmedStderr
	case trimmedStdout != "":
		return trimmedStdout
	default:
		return trimmedStderr
	}
}

func formatCommand(binaryPath string, args []string) string {
	parts := []string{strconv.Quote(binaryPath)}
	for _, arg := range args {
		parts = append(parts, strconv.Quote(arg))
	}
	return strings.Join(parts, " ")
}

func firstLine(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	lines := strings.Split(trimmed, "\n")
	return strings.TrimSpace(lines[0])
}
