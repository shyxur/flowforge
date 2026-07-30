#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "$script_dir/.." && pwd)"
cd "$repo_root"

usage() {
  cat <<'EOF'
Usage: ./scripts/time-startup.sh [--down | --fresh]

  --down   Stop existing containers before timing. Preserve named volumes.
  --fresh  Stop containers and delete named volumes before timing.
           This permanently removes local Compose data.

With no flag, measure an incremental `docker compose up --build -d` run.
EOF
}

mode="incremental"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --down)
      mode="down"
      ;;
    --fresh)
      mode="fresh"
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      printf 'error: unknown argument: %s\n' "$1" >&2
      usage >&2
      exit 2
      ;;
  esac
  shift
done

if ! command -v docker >/dev/null 2>&1; then
  printf 'error: required command not found: docker\n' >&2
  exit 1
fi

printf '==> Docker versions\n'
docker --version
docker compose version

if [[ "$mode" == "down" ]]; then
  printf '\n==> Stopping existing containers (volumes preserved)\n'
  docker compose down --remove-orphans
elif [[ "$mode" == "fresh" ]]; then
  printf '\n==> Removing existing containers and named volumes\n'
  docker compose down --volumes --remove-orphans
fi

printf '\n==> Timing docker compose up --build -d\n'
started_at="$(date +%s)"

if ! docker compose up --build -d; then
  elapsed_seconds=$(( $(date +%s) - started_at ))
  printf '\nstartup failed after %s seconds\n' "$elapsed_seconds" >&2
  docker compose ps --all || true
  exit 1
fi

elapsed_seconds=$(( $(date +%s) - started_at ))
printf '\nstartup completed in %s seconds\n' "$elapsed_seconds"

printf '\n==> Container status\n'
docker compose ps
