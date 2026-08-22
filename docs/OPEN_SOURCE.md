# Open-source project preparation

## Identity

- Display name: Redgres
- Repository slug: `redgres`
- Binary/service: `redgres`
- Tagline: “One secure control plane for PostgreSQL and Redis.”
- Proposed license: Apache-2.0

Before publishing, complete and record:

1. GitHub organization/repository availability.
2. Go module path and npm package-name availability if packages will be published.
3. Domain/social handle availability if desired.
4. Trademark search in intended jurisdictions and legal review appropriate to the project.
5. PostgreSQL and Redis trademark/naming-guideline review; make clear Redgres is independent and not endorsed by either project/company.

A quick search is not legal clearance.

## Required public repository assets

- Complete canonical license text and SPDX headers where appropriate.
- README with screenshots only after UI exists; never show real hosts/credentials/data.
- CONTRIBUTING, SECURITY, Code of Conduct, issue/PR templates.
- Release checksums, SBOM, changelog/release notes, and the maintained [supported-version matrix](COMPATIBILITY.md).
- License files/notices for any bundled Manrope, IBM Plex Mono, icon, or other design assets; production must not depend on third-party font CDNs.
- Architecture/security docs without live private infrastructure details.
- Example deployment profile using `example.com`; keep OneLife-specific profile in a clearly marked example or private overlay if operational sensitivity requires it.

## Provenance

Before copying Redis source, establish authorship/license/provenance because the supplied folder has no Git history. Before porting Python source, preserve copyright/license notices and check its dependency licenses. Document whether code is rewritten, copied, or adapted.

Do not publish real `.env`, SQLite/WAL files, binaries built from unknown source, certificates, tokens, backups, logs, production vault ciphertext, IP allow-lists, or customer/project names.

## Versioning

- Semantic Versioning after the first public release.
- `0.x` while APIs, installer, and migration behavior remain unstable.
- Release notes call out schema compatibility, migration, rollback constraints, Redis/PostgreSQL support, and security changes.
- Application/tool dependencies use latest-stable-compatible-at-review selection with exact lockfile/manifest pins; automated update proposals require human/agent review and complete gates before merge.
- Never promise stable API compatibility before a documented policy exists.
