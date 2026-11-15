# gx:agent-launch-and-recovery-2/06 - Peak process tree of gx check --full (PARTIAL)

Status: incomplete. Only the plain `go test` runs were measured, and only 2 of 3 were recorded. `gx check --full` was not run.

## Method

- Sampler: `ps -u $UID -o pid,ppid,comm` once per second, then a walk of descendants from the root with a visited set (same rule as `procwatch.Forest.Count`). The sampler is a throwaway script outside the repo.
- Command: `go test ./server/sessions/... ./ralphloop/agent/ ./testutil/... -count=1 -exec "$PWD/scripts/proc-cap"`.
- Nothing was killed. Safety abort was set at 1200 user processes (never reached).
- Baseline user process count was about 538, so proc-cap allowed about 738 (current + 200).

## Results

| Run | Peak | s > 200 | s > 300 | s > 500 | Top 5 at peak | Exit | Max user total |
|---|---|---|---|---|---|---|---|
| go test 1 | 166 | 0 | 0 | 0 | gx 78, claude 53, gxfixture 18, codex 8, (gx) 3 | 1 (FAIL) | 721 |
| go test 2 | 110 | 0 | 0 | 0 | banners.test 23, git 14, sessions.test 9, fixagent.test 9, promptflows.test 9 | 1 (FAIL) | 743 |
| go test 3 | not recorded (shell wrapper exited 1, no result file) | | | | | | |

## Findings

- Plain go test peaked at 166 and 110 here, below 200. The three logged kills (202-207) were not reproduced by this command.
- Both runs exited 1. The failure reason was not captured (output was cut with `tail -3`). Max user total reached 721 and 743, which is at or over the proc-cap limit (about 738), so a fork failure from the cap is a likely cause. This is unconfirmed.
- Peak "top names" differ a lot between runs (gx/claude helper processes vs test binaries), so one peak is not a stable number.
- Not measured: `gx check --full`, and a third go test run.
