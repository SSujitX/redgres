# OPS-009 live acceptance evidence

Store **redacted** operator evidence here after running [OPS-009-LIVE-ACCEPTANCE.md](../../agents/OPS-009-LIVE-ACCEPTANCE.md).

## Rules

- **Never commit** API tokens, OAuth secrets, tunnel tokens, certbot credentials, session cookies, or full zone IDs tied to production secrets.
- Redact: token values, `Authorization` headers, email addresses (use `owner@example.com` in docs), client IPs if sensitive.
- Filename pattern: `YYYY-MM-DD-<gate>-<short-description>.txt` (logs) or `.png` (screenshots, redacted).
- Each file starts with: `baseline_commit: <git SHA>`, `host: <hostname>`, `zone: <zone>`, `operator: <initials>`.

## Gates (execution order G1 → G9)

| Gate | Evidence file(s) | Pass |
|---|---|---|
| G1 Apply + Access | | ☐ |
| G2 OAuth scope pin | | ☐ |
| G3 cloudflared + console | | ☐ |
| G4 OAuth connect | | ☐ |
| G5 certbot (CF path only) | | ☐ |
| G6 confirm-reachable + UFW | | ☐ |
| G7 Playwright e2e | CI run link | ☐ |
| G8 Security sign-off | | ☐ |
| G9 Disconnect + Complete | | ☐ |

Git ignores all files in this directory except this README and `.gitkeep`.
