#!/usr/bin/env bash
set -uo pipefail

DEFAULT_ROOT="/opt/downloads/BangumiPipeline/data/bangumi"
ROOT="${BP_MEDIA_DIR:-$DEFAULT_ROOT}"
APPLY=0
FIX_ALL=0
WARNINGS_ONLY=0
KEEP_BACKUP=0
LOG_FILE=""
HAS_PYTHON=0

usage() {
	cat <<'USAGE'
Batch remux MP4 files for browser playback.

Default behavior is dry-run. Add --apply to replace files.

Usage:
  ./scripts/fix-mp4-remux.sh [options]

Options:
  --root PATH        Media root to scan.
                     Default: /opt/downloads/BangumiPipeline/data/bangumi
  --apply            Actually remux and replace matched files.
  --all              Remux every MP4 file instead of only suspicious files.
  --warnings-only    Only fix ffprobe edit-list warnings; skip slowstart checks.
  --keep-backup      Keep original file as .<name>.remux.bak after replacement.
  --log PATH         Append logs to PATH.
  -h, --help         Show this help.

The normal detection fixes:
  1. ffprobe edit-list/index warnings known to break browser playback.
  2. MP4 files whose moov box is after mdat, unless --warnings-only is used.
USAGE
}

log() {
	local level="$1"
	shift
	local line
	line="[$(date '+%F %T')] [$level] $*"
	printf '%s\n' "$line"
	if [[ -n "$LOG_FILE" ]]; then
		printf '%s\n' "$line" >>"$LOG_FILE"
	fi
}

die() {
	log "ERROR" "$*"
	exit 1
}

require_command() {
	local name="$1"
	if ! command -v "$name" >/dev/null 2>&1; then
		die "missing required command: $name"
	fi
}

while [[ $# -gt 0 ]]; do
	case "$1" in
		--root)
			shift
			[[ $# -gt 0 ]] || die "--root requires a path"
			ROOT="$1"
			;;
		--root=*)
			ROOT="${1#*=}"
			;;
		--apply)
			APPLY=1
			;;
		--all)
			FIX_ALL=1
			;;
		--warnings-only)
			WARNINGS_ONLY=1
			;;
		--keep-backup)
			KEEP_BACKUP=1
			;;
		--log)
			shift
			[[ $# -gt 0 ]] || die "--log requires a path"
			LOG_FILE="$1"
			;;
		--log=*)
			LOG_FILE="${1#*=}"
			;;
		-h | --help)
			usage
			exit 0
			;;
		*)
			die "unknown option: $1"
			;;
	esac
	shift
done

if [[ -n "$LOG_FILE" ]]; then
	mkdir -p "$(dirname "$LOG_FILE")" || die "failed to create log directory: $(dirname "$LOG_FILE")"
	touch "$LOG_FILE" || die "failed to write log file: $LOG_FILE"
fi

require_command ffmpeg
require_command ffprobe

if command -v python3 >/dev/null 2>&1; then
	HAS_PYTHON=1
fi

[[ -d "$ROOT" ]] || die "media root does not exist: $ROOT"

if [[ "$APPLY" -eq 0 ]]; then
	log "INFO" "dry-run mode; add --apply to replace files"
fi
log "INFO" "scanning root: $ROOT"

has_edit_list_warning() {
	local message="$1"
	printf '%s' "$message" | grep -qi 'edit list' || return 1
	printf '%s' "$message" | grep -Eqi 'Missing key frame|Cannot find an index entry' || return 1
	return 0
}

mp4_faststart_state() {
	local file="$1"
	if [[ "$HAS_PYTHON" -ne 1 ]]; then
		printf 'unknown\n'
		return 2
	fi
	python3 - "$file" <<'PY'
import os
import struct
import sys

path = sys.argv[1]
try:
    size_total = os.path.getsize(path)
    first_moov = None
    first_mdat = None
    offset = 0
    with open(path, "rb") as f:
        while offset < size_total:
            f.seek(offset)
            header = f.read(8)
            if len(header) < 8:
                break
            box_size, box_type = struct.unpack(">I4s", header)
            header_size = 8
            if box_size == 1:
                extended = f.read(8)
                if len(extended) < 8:
                    break
                box_size = struct.unpack(">Q", extended)[0]
                header_size = 16
            elif box_size == 0:
                box_size = size_total - offset
            if box_size < header_size:
                print("unknown")
                sys.exit(2)
            box_name = box_type.decode("latin1", "replace")
            if box_name == "moov" and first_moov is None:
                first_moov = offset
            elif box_name == "mdat" and first_mdat is None:
                first_mdat = offset
            if first_moov is not None and first_mdat is not None:
                break
            offset += box_size
    if first_moov is None or first_mdat is None:
        print("unknown")
        sys.exit(2)
    print("faststart" if first_moov < first_mdat else "slowstart")
except Exception:
    print("unknown")
    sys.exit(2)
PY
}

detect_reason() {
	local file="$1"
	local warning
	local faststart_state

	if [[ "$FIX_ALL" -eq 1 ]]; then
		printf 'forced by --all'
		return 0
	fi

	warning="$(ffprobe -v warning -i "$file" 2>&1 >/dev/null || true)"
	if has_edit_list_warning "$warning"; then
		printf 'ffprobe edit-list/index warning'
		return 0
	fi

	if [[ "$WARNINGS_ONLY" -ne 1 ]]; then
		faststart_state="$(mp4_faststart_state "$file" || true)"
		if [[ "$faststart_state" == "slowstart" ]]; then
			printf 'moov box is after mdat'
			return 0
		fi
	fi

	return 1
}

validate_output() {
	local file="$1"
	local duration

	if [[ ! -s "$file" ]]; then
		log "ERROR" "remux output is empty: $file"
		return 1
	fi

	duration="$(ffprobe -v error -show_entries format=duration -of default=nokey=1:noprint_wrappers=1 "$file" 2>/dev/null || true)"
	if [[ -z "$duration" || "$duration" == "N/A" ]]; then
		log "ERROR" "remux output has no readable duration: $file"
		return 1
	fi

	if ! ffprobe -v error -select_streams v:0 -show_entries stream=codec_name -of default=nokey=1:noprint_wrappers=1 "$file" >/dev/null 2>&1; then
		log "ERROR" "remux output has no readable video stream: $file"
		return 1
	fi

	return 0
}

remux_file() {
	local file="$1"
	local reason="$2"
	local dir
	local base
	local tmp
	local backup

	dir="$(dirname -- "$file")"
	base="$(basename -- "$file")"
	tmp="$dir/.$base.remux.$$.$RANDOM.tmp.mp4"
	backup="$dir/.$base.remux.bak"

	if [[ -e "$tmp" ]]; then
		log "ERROR" "temporary file already exists: $tmp"
		return 1
	fi
	if [[ -e "$backup" ]]; then
		backup="$dir/.$base.remux.$(date '+%Y%m%d%H%M%S').bak"
	fi

	log "INFO" "remuxing [$reason]: $file"
	if ! ffmpeg -hide_banner -nostdin -loglevel warning -y -i "$file" -map 0 -c copy -movflags +faststart "$tmp"; then
		log "ERROR" "ffmpeg failed: $file"
		rm -f -- "$tmp"
		return 1
	fi

	if ! validate_output "$tmp"; then
		rm -f -- "$tmp"
		return 1
	fi

	chmod --reference="$file" "$tmp" 2>/dev/null || true
	chown --reference="$file" "$tmp" 2>/dev/null || true
	touch -r "$file" "$tmp" 2>/dev/null || true

	if ! mv -f -- "$file" "$backup"; then
		log "ERROR" "failed to move original to backup: $file"
		rm -f -- "$tmp"
		return 1
	fi

	if ! mv -f -- "$tmp" "$file"; then
		log "ERROR" "failed to install remuxed file, restoring original: $file"
		mv -f -- "$backup" "$file" 2>/dev/null || true
		rm -f -- "$tmp"
		return 1
	fi

	if [[ "$KEEP_BACKUP" -eq 1 ]]; then
		log "INFO" "backup kept: $backup"
	else
		rm -f -- "$backup"
	fi

	log "INFO" "fixed: $file"
	return 0
}

scanned=0
candidates=0
fixed=0
failed=0
skipped=0

while IFS= read -r -d '' file; do
	scanned=$((scanned + 1))
	if reason="$(detect_reason "$file")"; then
		candidates=$((candidates + 1))
		if [[ "$APPLY" -eq 0 ]]; then
			log "INFO" "would fix [$reason]: $file"
		elif remux_file "$file" "$reason"; then
			fixed=$((fixed + 1))
		else
			failed=$((failed + 1))
		fi
	else
		skipped=$((skipped + 1))
	fi
done < <(find "$ROOT" -type f -iname '*.mp4' ! -name '*.remux.*.tmp.mp4' -print0)

log "INFO" "done: scanned=$scanned candidates=$candidates fixed=$fixed skipped=$skipped failed=$failed"

if [[ "$APPLY" -eq 0 && "$candidates" -gt 0 ]]; then
	log "INFO" "dry-run found $candidates candidate(s); rerun with --apply to modify files"
fi

if [[ "$failed" -gt 0 ]]; then
	exit 1
fi
