#!/usr/bin/env bash
# TLS/certbot inventory for installer --dry-run (OPS-009 Partial).
set -euo pipefail

redgres_inventory_tls() {
  redgres_log 'TLS inventory (read-only PATH scan; no secret read):'
  if redgres_resolve_host_binary certbot >/dev/null 2>&1; then
    redgres_log 'certbot: detected=path result=recorded'
  else
    redgres_log 'certbot: detected=missing result=recorded'
  fi
  if redgres_resolve_host_binary certbot >/dev/null 2>&1; then
    if certbot plugins 2>/dev/null | grep -q dns-cloudflare; then
      redgres_log 'certbot-dns-cloudflare: detected=plugin result=recorded'
    else
      redgres_log 'certbot-dns-cloudflare: detected=missing result=recorded'
    fi
  fi
}
