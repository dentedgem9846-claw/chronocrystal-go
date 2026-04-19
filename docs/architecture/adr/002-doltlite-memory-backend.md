# ADR-002: DoltLite as Memory Backend

**Status:** Accepted  
**Date:** 2026-04-19  
**Context:** ChronoCrystal needs persistent storage for conversations, messages, learnings, and skills. The data must survive process restarts and provide version history for debugging and recovery.

**Decision:** Use DoltLite as the memory backend, accessed via CGO with `mattn/go-sqlite3` and the `libsqlite3` build tag, linking against `libdoltlite.a`.

DoltLite is a SQLite fork with Git-like version control (prolly tree storage). It provides `dolt_commit`, `dolt_branch`, `dolt_diff`, `dolt_log`, and `dolt_reset` as SQL functions. Every meaningful state change triggers a `dolt_commit`.

**Alternatives Considered:**

1. **Plain SQLite** — Simpler, battle-tested, no CGO dependency. But no version history — debugging "what changed" requires custom audit tables. DoltLite gives this for free.
2. **PostgreSQL** — More powerful, but overkill for single-user local deployment. Requires separate process. DoltLite is embedded (same process).
3. **In-memory only** — No persistence across restarts. Unacceptable for an agent that must remember conversations.
4. **Flat files (JSON/Markdown)** — Simple but no query capability, no atomic writes, no version control.

**Consequences:**

- **Positive:** Full version history of every state change. `dolt_diff` shows exactly what changed between conversations. `dolt_reset` enables undo. `dolt_log` provides an audit trail. Same SQL interface as SQLite (drop-in for reads/writes). Embedded — no separate process.
- **Negative:** DoltLite is alpha software — may have bugs or incompatible format changes. CGO requirement complicates cross-compilation and static builds. Slightly slower writes than plain SQLite (prolly tree overhead). No remote push/pull (single-player only, which matches our single-user design).
- **Mitigation for DoltLite alpha:** Schema is standard SQL — fallback to plain SQLite requires only changing the driver and removing `dolt_commit` calls. The `Memory` interface abstracts this completely.