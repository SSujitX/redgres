#!/usr/bin/env bash
# Digest-pinned Redis images (same artifacts as ci.yml integration).
# PostgreSQL/Docker/PgBouncer come from Ubuntu + PGDG for the host codename.
set -euo pipefail

redgres_redis_image_pin() {
  case "$1" in
    8.2)
      printf '%s\n' 'redis:8.2.9@sha256:7d1e4ce8b9395088377ab382d1f6cfdbd13b3690795198a0399ab8d683064d6d'
      ;;
    8.8)
      printf '%s\n' 'redis:8.8.2@sha256:c514823c0ec1a40764df434efc2dc4ab5ec669c71c1cb00e4f7b1a694cee9fc3'
      ;;
    *)
      redgres_die "unsupported redis series pin ${1}"
      ;;
  esac
}
