# Test suite stability baseline

This is the number the test-suite-stability epic is measured against. It was written before the first fix landed, so no fix can move it.

## Baseline

109 of 111 recorded full `gx check` runs failed (about 98 percent).

- Date: 2026-11-17.
- Trunk commit: `6efc600bd6b2e5f12041cb67403bd6129cbf1b3e` (`main` when this file was written).
- Source: the `gx check` records, as counted in [test-failure-data.md](test-failure-data.md) on 2026-11-16.
- Per scope, from that report: `changed` 931 of 3,170 failed, `since` 90 of 138, `staged` 1 of 5, `full` 109 of 111.

Full runs include the Playwright flows.

## How to recompute

The records live in `<data dir>/check/gx.jsonl`, one JSON object per `gx check` run. The data dir is `$XDG_DATA_HOME/gx` or `~/.local/share/gx`. A full run has `scope` equal to `full`. It failed when `exit` is not 0.

```sh
jq -s '[.[]|select(.scope=="full")] | {total:length, failed:(map(select(.exit!=0))|length)}' ~/.local/share/gx/check/gx.jsonl
```

The command counts every full record in the file. It has no cutoff of its own, so the cutoff is the newest record at the time you run it.

The store keeps records for 30 days, so old records are swept. That is why the number is copied into this file.

## Live recount

The store kept growing after the report. The recount on 2026-11-17 counted full records from 2026-10-28T16:15:07Z to the cutoff 2026-11-17T15:55:51Z (the newest full record): 119 of 120 failed.

The baseline stays 109 of 111.

## What the number is not

The number counts runs, not first attempts. The brief index has no `attempt` field yet (ticket 03 adds it), so a rerun of a failed tree counts as another run. The count also mixes agent runs and release runs. Do not read it as a first-attempt pass rate.

## Context

The release checks on 2026-11-16 failed twice, with 23 and 14 failing tests.

## Related

- Research reports: `docs/research/test-*.md`, starting with [test-failure-data.md](test-failure-data.md).
- Spec: `docs/specs/test-suite-stability.md`.
- Done condition for the epic: 20 consecutive full runs that pass with no retry and no timeout change.
