# gx:agent-launch-and-recovery-2/06 - Peak process tree of gx check --full

## Method

- Sampler: `ps -u $UID -o pid,ppid,comm` once per second, then a walk of descendants from the root process with a visited set (same rule as `procwatch.Forest.Count`). A throwaway script outside the repo; nothing was killed. Safety abort at 1500 user processes (never reached).
- Roots: `go test ./server/sessions/... ./ralphloop/agent/ ./testutil/... -count=1 -exec "$PWD/scripts/proc-cap"` and `gx check --full` (gx applies its own cap).
- Machine was not idle (other gx sessions running, about 540 user processes at baseline). Run from the design worktree on 2026-11-15.

## Results

| Run | Peak | s > 200 | s > 300 | s > 500 | Top 5 at peak | Duration | Exit | Max user total |
|---|---|---|---|---|---|---|---|---|
| go test 1 | 170 | 0 | 0 | 0 | gx 78, claude 47, gxfixture 22, codex 8, git 5 | 170 s | 0 | 645 |
| go test 2 | 155 | 0 | 0 | 0 | gx 74, claude 49, gxfixture 16, codex 9, git 2 | 170 s | 1 | 714 |
| go test 3 | 158 | 0 | 0 | 0 | promptflows.test 37, askflows.test 23, prompt.test 23, sessions.test 21, banners.test 15 | 153 s | 1 | 700 |
| check --full 1 | 234 | 13 | 0 | 0 | gx 104, claude 66, codex 24, chrome-headless-shell 12, bash 6 | 595 s | 1 | 682 |
| check --full 2 | 235 | 11 | 0 | 0 | gx 103, claude 66, codex 24, chrome-headless-shell 12, bash 5 | 590 s | 1 | 682 |
| check --full 3 | 239 | 12 | 0 | 0 | gx 104, claude 66, codex 24, chrome-headless-shell 12, bash 5 | 596 s | 1 | 690 |

Exit 1 in `gx check --full` ("FAIL (1 failed)", same in all three runs) and in `go test` runs 2 and 3 (timing-sensitive banner and promptflows tests under load) was not diagnosed further. No log shows a process-cap error ("Resource temporarily unavailable"), and the peaks are complete samples of the whole run.

## Findings

- `gx check --full` peaks at 234-239, stable across runs. It is over 200 for 11-13 s in total and never reaches 300.
- Plain multi-package `go test` peaks at 155-170 here, under 200. The 205 kill in the logs was not reproduced.
- The peak is mostly gx, claude and codex helper processes of the test fixtures (gx ~104, claude ~66, codex ~24), not the build tools. A different machine load changes the mix, so the number moves a little.
- The three logged kills (202-207) are the watchdog cutting in at the start of this 11-13 s window. A limit of 500 is more than 2x the measured peak. A runaway fork reached 4,658 (see `scripts/proc-cap`).
- Longest unbroken stretch over 200: 13, 11 and 12 samples. A "N consecutive samples" rule needs N above 13 to let these runs pass.
