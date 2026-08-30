#!/bin/bash
# Runs cloudflared with TUNNEL_TOKEN from systemd LoadCredential.
# The token must never appear on the process argv or in unit Environment=.
set -euo pipefail
umask 077

CRED_FILE="${CREDENTIALS_DIRECTORY:?CREDENTIALS_DIRECTORY not set}/TUNNEL_TOKEN"
if [[ ! -f "$CRED_FILE" ]]; then
  echo "cloudflared-run: missing credential file" >&2
  exit 1
fi

# Trim trailing newlines only; do not log or echo the token.
TUNNEL_TOKEN="$(tr -d '\r\n' <"$CRED_FILE")"
if [[ -z "$TUNNEL_TOKEN" ]]; then
  echo "cloudflared-run: empty tunnel token" >&2
  exit 1
fi
export TUNNEL_TOKEN

CLOUDFLARED="${CLOUDFLARED_BIN:-}"
if [[ -z "$CLOUDFLARED" ]]; then
  if command -v cloudflared >/dev/null 2>&1; then
    CLOUDFLARED="$(command -v cloudflared)"
  else
    CLOUDFLARED="/usr/bin/cloudflared"
  fi
fi
# QUIC on this VPS edge flaps (Application error 0x0 → Error 1033). HTTP/2 is stable.
exec "$CLOUDFLARED" tunnel --no-autoupdate --protocol http2 run
