#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/.." && pwd)"

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
    cat <<'USAGE'
Create 560px / <=80,000-byte WebP sidecars for existing Bangumi covers.

Usage:
  bash scripts/optimize-bangumi-covers.sh [--root PATH] [--apply] [--force]
                                       [--ffmpeg PATH]

Default: preview only. --apply writes sidecars; --force regenerates them.
The root defaults to BP_COVER_DIR or <repo>/data/images/bangumi.
Only top-level numeric-ID JPG/JPEG/PNG/WebP files are selected.
Originals and SQLite are never modified. Failed files are reported and the
command exits nonzero; rerunning skips completed, current sidecars.
Requires Go (see backend/go.mod) and ffmpeg with libwebp for --apply.
Run as the service user, or give it read access to the resulting files.
USAGE
    exit 0
fi

cd "$REPO_ROOT"
exec go run ./backend/cmd/optimize-bangumi-covers "$@"
