#!/usr/bin/env bash
set -euo pipefail

branch="${1:-}"
base_ref="${2:-origin/main}"

if [[ -z "${branch}" ]]; then
  echo "Usage: $0 <branch> [base_ref]" >&2
  echo "Example: $0 codex/mairu-004-oauth-pkce origin/main" >&2
  exit 2
fi

if [[ "${branch}" != codex/* ]]; then
  echo "Error: branch name must start with 'codex/': ${branch}" >&2
  exit 2
fi

if [[ -n "$(git status --porcelain)" ]]; then
  echo "Error: working tree is not clean. Commit or stash changes first." >&2
  exit 1
fi

git fetch --all --prune

if git show-ref --verify --quiet "refs/heads/${branch}"; then
  echo "Error: branch already exists locally: ${branch}" >&2
  exit 1
fi

if ! git rev-parse --verify --quiet "${base_ref}" >/dev/null; then
  echo "Error: base ref not found: ${base_ref}" >&2
  exit 1
fi

git switch -c "${branch}" "${base_ref}"
echo "Created ${branch} from ${base_ref}"
