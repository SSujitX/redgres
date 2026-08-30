# ADR-014: Expert tool launch tickets

Status: Accepted
Date: 2026-08-30

## Context

pgAdmin and RedisInsight stay separate expert tools ([ADR-012](ADR-012-ui-bootstrap.md)). The Domain wizard publishes Tunnel + Access hostnames for them. Access email is not the Redgres owner session. A host-only console cookie is not sent to `pgadmin.*` or `redis.*`. SECURITY.md treats those URLs as hrefs and forbids Redgres from proxying or embedding them on GET `/status`.

## Decision

- The owner opens tools only from the authenticated Redgres UI.
- `POST /api/v1/tools/{pgadmin|redisinsight}/launch` (session + CSRF + `platform.network`) mints a one-time, short-lived ticket and returns a `launch_url` on that tool hostname. The raw ticket is not logged or audited.
- A loopback **tool gate** in `redgres serve` sits on the Tunnel origin ports and reverse-proxies to the container. It consumes `/__redgres/launch?ticket=` and sets an HttpOnly `SameSite=Strict` host-only tool cookie on that hostname. Requests without a valid cookie are not proxied.
- Redgres does not proxy the tools on the console origin and does not iframe them.
- Cloudflare Access remains the outer hostname gate.
- pgAdmin login email/password are revealed from a server-side file via `POST /api/v1/tools/pgadmin/credentials/reveal` (`no-store`); the UI clears them on dismiss, navigation, and selection change.
- After a valid tool cookie, the pgAdmin gate sets `X-Forwarded-User` / `Remote-User` to `REDGRES_PGADMIN_EMAIL` and strips any client-supplied values. The installer configures pgAdmin official webserver authentication so Open from Redgres skips the pgAdmin login form. RedisInsight has no separate login.
- The installer generates a pgAdmin master-password file and sets official `MASTER_PASSWORD_HOOK` to a script that prints that file ([config.py MASTER_PASSWORD_HOOK](https://www.pgadmin.org/docs/pgadmin4/9.17/config_py.html), [master password](https://www.pgadmin.org/docs/pgadmin4/9.17/master_password.html)). That supplies the vault key for webserver auth so the **Set Master Password** dialog is not required. Reveal returns login email/password and `master_password` from `REDGRES_PGADMIN_MASTER_PASSWORD_FILE`. pgAdmin does not persist the master password itself; Redgres stores the generated key only in that operator file.
- A tool-gate deny never redirects to bootstrap `:8989` or a raw public-IP HTTP origin. It uses the persisted `https://{console}` hostname when Domain apply has stored one.

## Consequences

- Tunnel ingress can keep pointing at `127.0.0.1:5050` / `5540` when those listeners are the gates and the containers bind other loopback ports.
- Full-stack install starts digest-pinned loopback compose for pgAdmin and RedisInsight and writes gate env keys. Domain apply persists the public tool URLs into `redgres.env` and the running process so System Open/Reveal work without a one-off host patch.
- A missing password file is `404` / not configured, never a leaked path.
