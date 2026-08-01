# 04 — Codex rate-limit recovery

**What to build:** Codex quota exhaustion pauses ralph-loop from structured
session data, reports the affected quota and reset time, and resumes the
affected iteration automatically once the reported reset occurs.

**Blocked by:** 02 — Codex smart-zone observability.

**Status:** claimed

- [ ] Exhausted Codex primary or secondary quota is distinguished from an
      ordinary attention request and from Claude's terminal-text rate limits.
- [ ] The loop stops scheduling, logs/shows the quota reason and reset time,
      and lets unrelated active iterations finish.
- [ ] The reset timestamp drives automatic recovery; malformed or unavailable
      quota data uses a safe polling fallback.
- [ ] The affected Codex pane is re-observed/re-prompted only when appropriate
      after quota recovery.
- [ ] Tests cover quota detection, timestamp reset, fallback polling, and no
      false positive on ordinary blocked state.
