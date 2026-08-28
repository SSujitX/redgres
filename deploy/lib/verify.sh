#!/usr/bin/env bash
# Fail-closed verify skip matrix (OPS-003 Partial).
# Never source, eval, or cat operator --config. Never mutate or probe.
# DNS / Cloudflare / public TLS remain skipped; this Partial is not Complete.
set -euo pipefail

redgres_verify_print_skip_matrix() {
  cat <<'EOF'
Verify (read-only --dry-run; not DNS/Cloudflare/public TLS; not Complete):
config: path-ok (unread, not sourced)
dns: skipped (this Partial cannot check DNS)
cloudflare: skipped (this Partial cannot check Tunnel/Access/routes)
tls_public: skipped (this Partial cannot check public certificates/TLS)
http_healthz: skipped (GET /api/v1/healthz not probed; curl not invoked)
auth_boundaries: skipped (GET /api/v1/status not probed)
bindings: skipped (live sockets deferred; intended redgres 127.0.0.1:8790)
bootstrap_ufw: skipped (intended allow 8989/tcp during bootstrap; removal via REDGRES_BOOTSTRAP_UFW_REMOVE_CMD)
cloudflared_unit: skipped (systemd unit presence not probed in dry-run)
services: skipped (cluster SHOW/INFO deferred; PATH --version is OPS-002 install)
backup_prerequisites: skipped (no named backup keys; OPS-004 owns backup)
result=partial
EOF
}

# config_path is already validated as an existing regular file; do not read it.
redgres_verify_dry_run() {
  redgres_verify_print_skip_matrix
}
