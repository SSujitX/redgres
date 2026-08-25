# Risk register

| ID | Risk | Likelihood/impact | Mitigation | Exit evidence |
|---|---|---|---|---|
| R-001 | Copied production Fernet records not proven decryptable in Go | Medium/Critical | In-process fixture decrypt exists in `internal/secrets` (Python `cryptography==49.0.0`); Gate 4 copied-record dry run and immutable secret backup remain | 100% sampled production records decrypt read-only |
| R-002 | PostgreSQL destructive policy targets system/shared data | Medium/Critical | Central protected policy, re-read, confirmations, integration/fault tests | Protected matrix passes with flags enabled |
| R-003 | Redis custom commands grant escalation or break apps | Medium/High | Explicit versioned allow-list + representative workloads | Supported Redis-series integration matrix |
| R-004 | One owner/login increases blast radius | Medium/High | Access outer layer, Argon2id, server sessions, reauth, audit, future capabilities | Security test/review |
| R-005 | Public raw DB ports attacked | High/High | TLS, SCRAM/ACL, source firewall, patching, monitoring | External negative/positive tests |
| R-006 | Existing installer damages PostgreSQL | Low/Critical | Separate modes, cluster identity guard, backup gate, VM clone rehearsal | Existing-mode invariant test |
| R-007 | Backup succeeds but restore fails | Medium/Critical | Checksums, complete RDB/AOF/ACL/SQLite procedure, isolated drills | Signed restore report |
| R-008 | Release rollback incompatible with migrated SQLite | Medium/High | Schema compatibility metadata, expand/contract migrations | Upgrade/rollback suite |
| R-009 | Runtime artifacts/secrets enter public Git | Medium/Critical | Ignore rules, provenance review, secret scanning, clean import | History/diff scan |
| R-010 | Legacy and Redgres port/route conflict | Medium/Medium | Stage on 8790, route inventory, listener checks | Coexistence verification |
| R-011 | Cross-store PostgreSQL password rotation leaves unknown secret | Medium/High | Operation state, retry/compensation, block concurrent rotation, incident path | Fault-injection tests |
| R-012 | Externally managed Redis ACL rewritten incorrectly | Medium/High | Detect unrepresentable rules, read-only/adoption gate | Import/adoption tests |
| R-013 | One-host resource contention harms databases | Medium/High | Live sizing, Redis maxmemory, PG tuning, resource limits, capacity alerts | Load/capacity report |
| R-014 | Documentation diverges from code | High/Medium | Traceability, generated config/API refs, PR checks | CI/doc review evidence |
| R-015 | Project name conflicts legally | Unknown/Medium | Formal availability/trademark check before launch | Recorded approval |
| R-016 | Untested service version or floating artifact changes administrative behavior | Medium/Critical | Release-owned compatibility matrix, exact package/image/digest pinning, runtime detection/capability checks, no implicit major/series upgrade | Complete matrix CI plus fresh/adoption and restore evidence |
| R-017 | Desktop-only or generic UI hides context/actions on smaller devices or confuses PostgreSQL/Redis scope | Medium/High | Shared UI contract/tokens, explicit shell modes, viewport/zoom/browser tests, accessibility checks, independent UI reviewer | NFR-012 evidence at required viewports and no critical/high UI findings |
| R-018 | PostgreSQL capability plan upgrades packages, overwrites preload settings, restarts unexpectedly, or enables extensions in the wrong database | Medium/Critical | Existing-mode preserve default, release-owned capability registry, named database scope, config merge/diff, backup/capacity/restart approval, direct-path and restore tests | OPS-007 evidence for each claimed PostgreSQL-major/capability combination |

Owners and due dates are assigned in the issue tracker when implementation begins. “Accepted” risks require explicit maintainer/operator approval, not silence.
