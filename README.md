# Mairu

Desktop Gmail cleaner built with Wails (Go + React). The product vision, architecture, and feature roadmap live in [`docs/gmail_cleaner_design_v2.md`](docs/gmail_cleaner_design_v2.md).

## Current Status
- ✅ Design docs imported from `gmail_cleaner_design_v2.docx`.
- ⏳ Source code not generated yet. Next step is scaffolding a Wails v2 project that matches the structure described in the design doc.

## Prerequisites (when development begins)
- Go 1.22+
- Node.js 20+ (or the version required by Wails React template)
- Wails CLI (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`)
- pnpm or npm for frontend package management

## Recommended Workspace Layout
```
mairu/
├── frontend/         # React + Tailwind app (Wails template)
├── internal/         # Go packages (gmail, claude, db, auth, ...)
├── app.go            # Wails bindings exposed to the UI
├── main.go           # Wails entry point
└── docs/             # Design + development docs
```

## Next Steps
1. Run `wails init` (React + Tailwind template) at the repo root.
2. Move/rename generated files to match the structure above.
3. Start translating each section of the design doc into concrete Go/React modules.

For detailed workflow notes (especially when collaborating with Codex), see [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md).
