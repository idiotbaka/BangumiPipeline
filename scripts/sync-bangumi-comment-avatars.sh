#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/.." && pwd)"
DATABASE_PATH="${BP_DATABASE_PATH:-$REPO_ROOT/data/bangumi-pipeline.db}"
COVER_DIR="${BP_COVER_DIR:-$REPO_ROOT/data/images/bangumi}"
OUTPUT_DIR="$COVER_DIR/avatar"
BATCH_SIZE=50

usage() {
	cat <<'USAGE'
Backfill locally cached medium avatars for existing Bangumi episode comments.

Usage:
  ./scripts/sync-bangumi-comment-avatars.sh [options]

Options:
  --database PATH   SQLite database path. Defaults to BP_DATABASE_PATH or
                    <repo>/data/bangumi-pipeline.db.
  --output PATH     Avatar output directory. Defaults to
                    BP_COVER_DIR/avatar or <repo>/data/images/bangumi/avatar.
  --batch NUMBER    Sequential download batch size (1-500, default 50).
  -h, --help        Show this help.

The command reads the HTTP/HTTPS proxy and Bangumi User-Agent from the same
application configuration used by the server. Existing avatars are reused by
Bangumi user ID; failed downloads retain retry state in SQLite.
USAGE
}

while [[ $# -gt 0 ]]; do
	case "$1" in
		--database)
			shift
			[[ $# -gt 0 ]] || { echo "--database requires a path" >&2; exit 2; }
			DATABASE_PATH="$1"
			;;
		--database=*) DATABASE_PATH="${1#*=}" ;;
		--output)
			shift
			[[ $# -gt 0 ]] || { echo "--output requires a path" >&2; exit 2; }
			OUTPUT_DIR="$1"
			;;
		--output=*) OUTPUT_DIR="${1#*=}" ;;
		--batch)
			shift
			[[ $# -gt 0 ]] || { echo "--batch requires a number" >&2; exit 2; }
			BATCH_SIZE="$1"
			;;
		--batch=*) BATCH_SIZE="${1#*=}" ;;
		-h|--help) usage; exit 0 ;;
		*) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
	esac
	shift
done

[[ -f "$DATABASE_PATH" ]] || { echo "database not found: $DATABASE_PATH" >&2; exit 2; }

cd "$REPO_ROOT/backend"
exec go run ./cmd/sync-bangumi-comment-avatars \
	--database "$DATABASE_PATH" \
	--output "$OUTPUT_DIR" \
	--batch "$BATCH_SIZE"
