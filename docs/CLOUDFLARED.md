# Cloudflare Tunnel (cloudflared) wiring for Redgres

Status: **OPS-009 Partial** — apply writes remote ingress + connector token; systemd units are in-repo. Installing the `cloudflared` package is still an operator step (not yet automated by `deploy/install.sh`). No live Cloudflare/`cloudflared` e2e evidence in CI.

Primary Cloudflare references:

- [Create a tunnel (API)](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/get-started/create-remote-tunnel-api/) — `config_src: "cloudflare"`, then `PUT …/cfd_tunnel/{id}/configurations` ingress, then DNS CNAME to `<tunnel-id>.cfargotunnel.com`
- [Downloads / Linux packages](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/downloads/) and [pkg.cloudflare.com](https://pkg.cloudflare.com/)
- Prefer `TUNNEL_TOKEN` env over `--token` on argv (avoids process-list exposure; Wrangler/tunnel guidance)

## What Redgres already does on apply

`POST /api/v1/domain/apply` (after a per-zone API token is stored):

1. Creates a **remotely managed** tunnel (`config_src=cloudflare`).
2. Sets tunnel **ingress**: `hostname` → `http://127.0.0.1:<port>` where `<port>` comes from `REDGRES_ADDRESS` (always loopback origin on this host).
3. Creates the proxied DNS CNAME to `<tunnel-id>.cfargotunnel.com`.
4. Creates a deny-by-default Access app for the hostname (you still must add an allow policy).
5. Writes the one-time connector token to `REDGRES_TUNNEL_TOKEN_FILE` (mode `0600`, never returned by the API).

Without `cloudflared` running with that token, the public hostname will not reach Redgres.

## Exact server setup (Ubuntu)

Run as root on the Redgres host. Paths below match production defaults; if you change `REDGRES_TUNNEL_TOKEN_FILE`, edit the unit `LoadCredential=` and path unit paths to match.

### 1. Paths and Redgres env

```bash
install -d -m 0700 -o redgres -g redgres /var/lib/redgres/secrets
```

Ensure Redgres is configured (example):

```bash
# REDGRES_ADDRESS=127.0.0.1:8790
# REDGRES_CLOUDFLARE_TOKEN_FILE=/var/lib/redgres/secrets/cloudflare-api-token
# REDGRES_TUNNEL_TOKEN_FILE=/var/lib/redgres/secrets/cloudflared-tunnel-token
```

Canonical bind must already answer on loopback before the tunnel is useful:

```bash
curl -sS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8790/api/v1/healthz
```

### 2. Install cloudflared (official apt repo)

Use Cloudflare’s package repository ([pkg.cloudflare.com](https://pkg.cloudflare.com/)). Prefer the **codename** matching your Ubuntu release (`noble` = 24.04, `jammy` = 22.04, `focal` = 20.04). The `any` suite also works on Debian-based systems.

**Ubuntu 24.04 (noble):**

```bash
mkdir -p --mode=0755 /usr/share/keyrings
curl -fsSL https://pkg.cloudflare.com/cloudflare-main.gpg | tee /usr/share/keyrings/cloudflare-main.gpg >/dev/null
echo 'deb [signed-by=/usr/share/keyrings/cloudflare-main.gpg] https://pkg.cloudflare.com/cloudflared noble main' \
  | tee /etc/apt/sources.list.d/cloudflared.list
apt-get update && apt-get install -y cloudflared
command -v cloudflared
cloudflared --version
```

If the package installed a default `cloudflared.service`, do **not** use it with a dashboard token pasted on argv. Disable it so only Redgres’s unit runs:

```bash
systemctl disable --now cloudflared.service 2>/dev/null || true
```

### 3. Install Redgres unit helpers from this repo

From a checked-out release tree on the server:

```bash
install -d -m 0755 /usr/libexec/redgres
install -m 0755 deploy/systemd/cloudflared-run.sh /usr/libexec/redgres/cloudflared-run.sh
install -m 0644 deploy/systemd/cloudflared-redgres.service /etc/systemd/system/cloudflared-redgres.service
install -m 0644 deploy/systemd/cloudflared-redgres-restart.service /etc/systemd/system/cloudflared-redgres-restart.service
install -m 0644 deploy/systemd/cloudflared-redgres.path /etc/systemd/system/cloudflared-redgres.path
systemctl daemon-reload
```

How the connector gets the token:

1. Apply writes the plaintext connector token to `/var/lib/redgres/secrets/cloudflared-tunnel-token` (`0600` via `securefile`). The token **does** live on disk by design; keep the parent directory `0700` root-owned.
2. `cloudflared-redgres.service` uses `LoadCredential=TUNNEL_TOKEN:<that path>` (systemd copies into `$CREDENTIALS_DIRECTORY/TUNNEL_TOKEN`).
3. `cloudflared-run.sh` exports `TUNNEL_TOKEN` and execs `cloudflared tunnel --no-autoupdate run` — **no `--token` on argv** (avoids process-list exposure). “Never on argv” does **not** mean the token exists only inside systemd.
4. `cloudflared-redgres.path` watches the token file; on create/change it starts `cloudflared-redgres-restart.service`, which `systemctl restart`s the connector (plain `Unit=cloudflared-redgres.service` would **not** restart an already-running connector). On a first-ever apply this is effectively a **start** (`restart` starts an inactive unit).

### 4. Enable path watcher (and connector after apply)

```bash
systemctl enable --now cloudflared-redgres.path
```

Order relative to Domain apply:

1. Paste API token + run apply in the UI/API (creates tunnel, ingress, DNS, Access app, token file).
2. Path unit sees the token file and restarts/starts the connector — **or** start explicitly:

```bash
systemctl start cloudflared-redgres.service
systemctl status cloudflared-redgres.service --no-pager
journalctl -u cloudflared-redgres.service -n 50 --no-pager
```

### 5. Verify

```bash
# Tunnel should show healthy / connections in Zero Trust → Networks → Tunnels
# Public hostname (Access may still deny until you add an allow policy):
curl -sS -o /dev/null -w '%{http_code}\n' https://redgres.example.com/
```

Expect Access challenge / deny until a policy exists — that still proves the tunnel hop works if Cloudflare returns Access HTML rather than a tunnel error.

### 6. Access policy (required for browser login)

Apply creates an Access **application** only (deny by default). Use Domain & Network steps **Access allow policy** then **Console is reachable — close bootstrap** (`POST /api/v1/domain/access-policy`, `POST /api/v1/domain/confirm-reachable`) before treating the domain as steady-state. Bootstrap `:8989` stays open until confirm succeeds (or the hard-cap TTL fires).

### 7. Firewall

- cloudflared needs **outbound** reachability to Cloudflare: default is QUIC over **UDP 7844**, with fallback to **HTTP/2 over TCP 443**. Strict egress must keep **443** reachable too (or pin `--protocol`); do not allow only 7844 and then wonder why the connector fails.
- It does **not** need a public inbound UI port.
- Keep Redgres canonical bind on `127.0.0.1:8790` (or your configured loopback). Do not open `:8790` publicly.

## Disconnect

`DELETE /api/v1/domain` deletes Cloudflare resources Redgres created and removes the tunnel token file. The path unit does **not** stop the connector when that file disappears — stop it explicitly:

```bash
systemctl stop cloudflared-redgres.service
```

## Security rules

- Never put the tunnel token in unit `Environment=`, shell history, or long-lived `cloudflared … --token …` commands.
- Never commit token files. Protect `/var/lib/redgres/secrets` (`0700`) and the token file (`0600`).
- Do not share one Cloudflare OAuth app / API token across unrelated customer domains; each install uses its own per-zone token.
