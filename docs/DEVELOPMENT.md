# Development Guide (Codex Friendly)

This repo is optimized for working with Codex CLI agents. Follow the flow below to keep tasks predictable and low-friction.

## 1. Task Intake
1. Gather context:
   - Read [`docs/gmail_cleaner_design_v2.md`](gmail_cleaner_design_v2.md).
   - Check `README.md` for the current implementation status.
2. Define the goal explicitly in the user request (e.g., "Create Gmail client skeleton" rather than "help me").
3. When work spans multiple files, let the agent create a plan (`/plan` action in Codex CLI) so changes stay organized.

## 2. Local Environment
- Install Go 1.22+, Node.js 20+, and the Wails CLI.
- Preferred package manager: `pnpm` (falls back to `npm` if unavailable).
- Recommended helper commands (create later):
  - `make dev` → Run `wails dev` for live reload.
  - `make build` → Run platform builds via `wails build`.
  - `make test` → Run `go test ./...` and frontend unit tests.

Until a `Makefile` exists, run the commands directly.

## 3. Directory Conventions
- `app.go` exposes Go methods to the React frontend via Wails bindings.
- `internal/gmail`, `internal/claude`, `internal/db`, `internal/auth` host service-specific code.
- `frontend/src/pages/*` should align with the feature areas (Classify, Blocklist, Export, Migration, Settings).
- Keep cross-cutting types (mail models, shared DTOs) under `internal/types` once created.

## 4. Working With Codex CLI
- **Use short commands**: prefer `rg`, `ls`, `sed` over heavier alternatives.
- **Never revert user changes** unless explicitly requested.
- **Quote files** when referencing them in responses (`path/to/file:42`).
- **Tests**: describe how to verify results whenever the agent cannot run them.
- **Networking**: sandbox is restricted; ask for approval only when unavoidable.

## 5. Branch & Commit Strategy
- Default to the `main` branch until additional branches are required.
- Squash commits per feature if the PR workflow prefers it; otherwise, keep each logical change atomic.
- Include the feature/bug identifier in commit messages when available (e.g., `feat: add Gmail client skeleton`).

## 6. Quality Checklist Before Finishing a Task
- [ ] Run `go fmt ./...` and `go test ./...` (once code exists).
- [ ] Run `pnpm lint` / `pnpm test` for the frontend (once configured).
- [ ] Ensure docs mention new config/env vars.
- [ ] Update `README` or `docs/` when architecture or workflows change.
- [ ] Attach verification steps in the final Codex response.

## 7. Useful References
- [Wails Docs](https://wails.io/docs/introduction)
- [Google Gmail API for Go](https://pkg.go.dev/google.golang.org/api/gmail/v1)
- [Claude API](https://docs.anthropic.com/)

Update this guide as tooling or workflows evolve so future Codex sessions stay smooth.
