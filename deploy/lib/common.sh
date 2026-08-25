#!/usr/bin/env bash
# Shared installer helpers (OPS-001/002 Partial). Do not source operator --config.
set -euo pipefail

redgres_log() {
  printf '%s\n' "$*"
}

redgres_die() {
  printf '%s\n' "$*" >&2
  exit 1
}

redgres_not_implemented() {
  printf '%s\n' "$*" >&2
  exit 2
}
