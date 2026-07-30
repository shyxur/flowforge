#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "$script_dir/.." && pwd)"
cd "$repo_root"

section() {
  printf '\n==> %s\n' "$1"
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'error: required command not found: %s\n' "$1" >&2
    exit 1
  fi
}

section "checking required commands"
require_command go
require_command docker

section "running Go tests"
go test ./...

section "running Go vet"
go vet ./...

section "validating Docker Compose configuration"
docker compose config --quiet

if [[ -d apps/dashboard && -f apps/dashboard/package.json ]]; then
  require_command npm

  section "linting dashboard"
  (
    cd apps/dashboard
    npm run lint
  )

  section "building dashboard"
  (
    cd apps/dashboard
    npm run build
  )
fi

section "smoke test passed"
