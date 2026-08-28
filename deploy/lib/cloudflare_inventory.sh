#!/usr/bin/env bash
# Cloudflare inventory for installer --dry-run (OPS-009 Partial).
set -euo pipefail

redgres_inventory_cloudflare() {
  redgres_log 'Cloudflare inventory (read-only PATH scan; not live API):'
  if ( redgres_resolve_host_binary cloudflared >/dev/null 2>&1 ); then
    redgres_log 'cloudflared: detected=path result=recorded'
  else
    redgres_log 'cloudflared: detected=missing result=recorded'
  fi
}
