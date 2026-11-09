# pr-flow/04: What gh gives the server watcher

Question: what does `gh` give the gx server to watch a PR without an agent?

Sources: the `gh` manual (`gh <cmd> --help`, cli.github.com/manual, gh 2.103.3), GitHub REST and GraphQL docs on docs.github.com, gx's `git/pr/*.go`, and read-only `gh` calls against public `cli/cli` PR 14976 (merged) on 2026-11-09. Nothing was created or edited on GitHub.

Marks: **[doc]** stated in docs, **[run]** observed with a local read-only call, **[open]** not settled by docs.

## 1. Draft PR and ready

- Open a draft: `gh pr create --draft --head <branch> --base <trunk> --title <t> --body-file <f>`. It prints the PR URL on success. [doc: `gh pr create --help`]
- Pass `--head` so `gh` never prompts or forks. Without `--head`, an unpushed branch triggers a "where to push" prompt. The server must push the branch itself first (`git push`), since `gh` only pushes interactively. [doc]
- Pass `--title` and `--body` (or `--body-file`) to skip the prompts. [doc]
- A body containing `Fixes #123` or `Closes #123` closes that issue on merge. gx should not write those into the body by accident. [doc]
- Mark ready: `gh pr ready [<number> | <url> | <branch>]`. `--undo` converts back to draft. [doc]
- Under the hood, ready is GraphQL mutation `markPullRequestReadyForReview(input:{pullRequestId})`, returning `pullRequest`. [doc: GraphQL reference, pulls]
- `gh pr ready` on a PR that is already ready: behavior not documented. [open] Check `isDraft` first.
- Use `-R OWNER/REPO` on every call, so the server does not depend on its cwd. [doc]

## 2. PR state

`gh pr view <n> -R o/r --json state,isDraft,mergeable,mergeStateStatus,headRefOid,baseRefName,mergedAt,mergeCommit,closedAt`. All of these are listed JSON fields of `gh pr view`. [doc]

- `state`: `OPEN`, `CLOSED`, `MERGED`. [run: `MERGED` seen. doc lists PullRequestState without values on the fetched page.]
- Merged vs closed unmerged: `state` is `MERGED` only for a merge. `closed` without merge is `CLOSED`. GraphQL also has `merged` (Boolean!) and `mergedAt`. [doc]
- Merge commit: `mergeCommit.oid` (JSON field `mergeCommit`). REST `merge_commit_sha` after merge is: the merge commit (merge), the squashed commit on the base branch (squash), or the commit the base was updated to (rebase). So a squash merge gives one new commit on trunk that is NOT the PR head SHA. [doc: REST pulls]
  [run] PR 14976 returned `mergeCommit: {"oid":"736c68f..."}` while `headRefOid` was `6ceb78a...`.
- Consequence for gx: after a squash merge, `git merge-base --is-ancestor <head> trunk` is false. Detect merge by `state == MERGED`, not by ancestry. `IsCommitMergedToTrunk` in `git/pr/pr.go` cannot detect a squash merge. For a local fast-forward after merge, fetch and use `mergeCommit.oid`.
- Before merge, REST `merge_commit_sha` is the test merge commit, not a real commit. Do not use it. [doc]
- `mergeable`: GraphQL `MergeableState` is `CONFLICTING`, `MERGEABLE`, `UNKNOWN`. `UNKNOWN` means "still being calculated". [doc]
  REST: `mergeable` is `true`, `false`, or `null`. `null` means a background job started, so ask again later. GitHub starts the job when the PR is fetched, created or edited. [doc]
  [run] A merged PR reports `mergeable: UNKNOWN`, `mergeStateStatus: UNKNOWN`. Only read these on `OPEN` PRs.
- `mergeStateStatus` (GraphQL `MergeStateStatus`): `BEHIND` (head out of date), `BLOCKED`, `CLEAN`, `DIRTY` (merge commit cannot be created, i.e. conflicts), `DRAFT` (deprecated, use `isDraft`), `HAS_HOOKS`, `UNKNOWN`, `UNSTABLE` (mergeable, non-passing status). [doc]
- Conflict detection for the Merge ticket: `mergeable == CONFLICTING` (or `mergeStateStatus == DIRTY`). Treat `UNKNOWN` as "ask again next poll", never as clean. `BEHIND` is not a conflict. It only means the branch lacks trunk commits (relevant if the repo requires up-to-date branches).
- Head SHA: `headRefOid`. Base: `baseRefName`, `baseRefOid`. The head SHA changes on every push, so it is the key for "CI results belong to this push". [doc]
- Closed without merge: `state == CLOSED`, `mergedAt` null. [doc]
- A person can also turn on GitHub auto-merge: `autoMergeRequest` is a listed field (non-null when enabled). [run: field present on PR 14976.] Out of scope for gx, but the watcher still sees `MERGED`.

## 3. CI for the head commit

Two views. The server should use the GraphQL one for ids, and `gh pr checks` only for a quick look.

### 3a. `gh pr checks <n> -R o/r --json ...` [doc]

Fields: `bucket, completedAt, description, event, link, name, startedAt, state, workflow`.

- `bucket` groups `state` into `pass`, `fail`, `pending`, `skipping`, `cancel`.
- Exit code 8 means checks pending. Exit code 0 all pass. Exit 1 failure. (`gh help exit-codes` lists 0, 1, 2 cancelled, 4 needs auth; `pr checks` adds 8.) [doc]
- `--required` limits to required checks. `--watch` blocks. The server should not use `--watch`.
- It has NO run id or job id. The `link` is a URL: for Actions it is `.../actions/runs/<run_id>/job/<job_id>`. [run] For a non-Actions check (e.g. a code scanning check) the link is `.../runs/<check_run_id>`. [run]
- `workflow` is empty for non-Actions checks. [run]

### 3b. `gh pr view --json statusCheckRollup` [doc + run]

Returns a mixed list. Each entry has `__typename`:

- `CheckRun`: `name`, `status`, `conclusion`, `startedAt`, `completedAt`, `detailsUrl`, `workflowName`. [run]
- `StatusContext` (legacy commit status): `context`, `state`, `targetUrl` (GraphQL names). [doc: commit statuses]

There is no run id or job id field in this JSON. Parse it from `detailsUrl`, or use GraphQL (3c).

### 3c. GraphQL, the best source (one call) [run, tested on PR 14976]

```graphql
query($o:String!,$n:String!,$p:Int!){
  repository(owner:$o,name:$n){
    pullRequest(number:$p){
      state merged mergedAt mergeCommit{oid} headRefOid
      mergeable mergeStateStatus isDraft
      commits(last:1){ nodes{ commit{ oid
        statusCheckRollup{ state
          contexts(first:100){ totalCount
            pageInfo{hasNextPage endCursor}
            nodes{ __typename
              ... on CheckRun{ databaseId name status conclusion detailsUrl
                checkSuite{ databaseId workflowRun{ databaseId runNumber event
                  workflow{ databaseId name } } } }
              ... on StatusContext{ context state targetUrl } } } } } } }
    }
  }
}
```

- `statusCheckRollup.state`: `SUCCESS`, `FAILURE`, `ERROR`, `PENDING`, `EXPECTED`. `git/pr/pr_list.go` already maps these. Null when the commit has no checks.
- `CheckRun.databaseId` is the job id for an Actions job: [run] `detailsUrl` ended `/job/128259599242` and `databaseId` was `128259599242`. Same number.
- `checkSuite.workflowRun.databaseId` is the run id: [run] `42325466453`, equal to the `/runs/<id>/` in `detailsUrl`. It is null for a check that does not come from Actions (not tested; the field is nullable in the schema).
- `workflowRun.workflow.databaseId` and `.name` identify the workflow file.
- `contexts` is a connection, max 100 per page. `totalCount` was 11 for the test PR. Page with `endCursor` if `hasNextPage`.
- `CheckConclusionState` values: `ACTION_REQUIRED, CANCELLED, FAILURE, NEUTRAL, SKIPPED, STALE, STARTUP_FAILURE, SUCCESS, TIMED_OUT`. `CheckStatusState`: `COMPLETED, IN_PROGRESS, PENDING, QUEUED, REQUESTED, WAITING`. [doc: GraphQL checks]
- Gap in gx: `failedCheckConclusions` in `git/pr/pr.go` lacks `STARTUP_FAILURE`. The server should treat `STARTUP_FAILURE` as failed. It also counts `CANCELLED` as failed, which is wrong for a run cancelled by a newer push (superseded). See 3e.
- `StatusContext.state` values: `error`, `failure`, `pending`, `success` (REST); GraphQL `StatusState` has `EXPECTED, ERROR, FAILURE, PENDING, SUCCESS`. [doc]

### 3d. Check identity across reruns and pushes (for per-check attempt counts)

What is documented:

- A check run has `name`. Within a check suite, up to 1,000 check runs may share a name. [doc: REST check runs]
- The list endpoints default to `filter=latest`, which returns the most recent run per name by `completed_at`. `filter=all` returns all. So a rerun appears as another check run with the same name in the same suite. [doc]
- A rerun of Actions jobs keeps the same workflow run id and raises `run_attempt`. Jobs list defaults to `filter=latest` (jobs of the latest execution), `all` includes earlier executions. The run object has `run_attempt` and `previous_attempt_url`. [doc: REST workflow runs, workflow jobs]
- A rerun is limited to 50 per run and 30 days. [doc]
- A new push makes new workflow runs (new run ids) and a new head SHA. The old run ids belong to the old SHA. [doc by implication; see [open] below]

Recommended identity key for "the same check": `(workflow.databaseId, check name)` for Actions checks, `("status", context)` for legacy statuses, and `(app, name)` for other check runs. The matrix job name already contains the matrix values (for example `build (windows-latest)` [run]), so `name` is unique per matrix cell. Do NOT key on `databaseId` (job id), run id or head SHA: those all change across reruns or pushes.

Counting: store `{key -> attempts}`. Bump a key when gx starts a fix agent for a failing check on a head SHA it has not already handled. A key seen failing again on a new head SHA counts as the next attempt.

[open] These are not stated in the docs, and gx should verify them in a sandbox repo before relying on them:
- Whether a job in a rerun keeps the old job id or gets a new one. (The jobs list having a `latest` filter suggests the same job name appears again, but the docs do not give ids.)
- Whether a renamed job, or a job gated by a changed matrix, appears as a new key. It does.
- What `workflowRun` is for a required status that never ran (state `EXPECTED`).
- Whether the same `name` can come from two different workflows in one PR. It can: `workflow.databaseId` in the key keeps them apart.

### 3e. Superseded runs

When a new push lands, runs for the old SHA may end `CANCELLED` (for example with a `concurrency: cancel-in-progress` group). [doc: concurrency is a standard Actions feature; not fetched here] Because the rollup is read for the PR's current head only (`commits(last:1)`), cancelled runs from old SHAs never show up. A `CANCELLED` conclusion on the current head came from a person or a concurrency cancel on a newer run that may not be on the PR yet. gx should not start a fix agent for `CANCELLED`. `pr.go` currently counts it as failed. [open: no doc says which cancel reasons occur on the head commit]

## 4. Failure logs

- `gh run view <run_id> --log-failed -R o/r`: logs of failed steps of a run. `gh run view <run_id> --job <job_id> --log-failed`: one job. `--attempt <n>` picks an attempt. `gh run view <id> --log`: everything. [doc: `gh run view --help`]
- `gh run view --json jobs,attempt,conclusion,status,headSha,url,...`. Fields: `attempt, conclusion, createdAt, databaseId, displayTitle, event, headBranch, headSha, jobs, name, number, startedAt, status, updatedAt, url, workflowDatabaseId, workflowName`. [doc] Each job has `databaseId, name, status, conclusion, url` (and steps). [run: job0 printed `databaseId, name, conclusion, status, url`]
- `gh pr checks` has no log option. To get a log from a PR check, take the run id and job id from the GraphQL query in 3c, then call `gh run view`.
- How gh fetches logs: it downloads the run's zip first. If it cannot match jobs to logs, it falls back to one API call per job. If more than 25 job logs are missing, it fails. Some lines may show step name `UNKNOWN STEP`. [doc: `gh run view --help`]
- REST endpoints behind it: `GET /repos/{o}/{r}/actions/runs/{run_id}/logs` (zip), `.../runs/{run_id}/attempts/{n}/logs`, `GET .../actions/jobs/{job_id}/logs` (plain text). Each answers 302 to a download URL that expires after 1 minute. Read access is enough. [doc]
- Size limits and log retention: NOT stated on the fetched REST pages or the Actions limits page. [open] The check run `output.summary` and `output.text` cap at 65,535 characters each, and annotations cap at 50 per request (Actions: 10 warnings and 10 errors per step). Those are limits of the check run output, not of job logs. [doc]
- Logs of a run are available only after the run completes. `gh run view --log` on an in-progress run: not documented. [open] Only ask for logs when the check's `status` is `COMPLETED`.
- A run can still be rerun for 30 days. Run logs for old runs may be deleted (`DELETE .../logs` exists), so treat a missing log as a recoverable error. [doc: the delete endpoint exists; retention not stated]
- Plan for the fix agent: pass the run id and job id, and let the agent run `gh run view --job <id> --log-failed` itself, tail-trimmed. The server only needs to know which checks failed.
- [run] `gh run view <run> --log-failed` on a run with no failed steps printed nothing and exit 0.

## 5. Polling cost

- Primary limits: 5,000 requests per hour for a user token, which `gh` uses (REST). `GITHUB_TOKEN` in Actions is 1,000 per repo per hour. GraphQL has its own 5,000 points per hour for a user. [doc]
  [run] `gh api rate_limit` showed separate buckets: `core` 5000, `graphql` 5000, `search` 30 per minute.
- GraphQL cost: sum of requests needed for all connections at their `first`/`last` maxima, divided by 100, minimum 1. Max 100 per connection, max 500,000 nodes. [doc] Observed: the query in 3c with `contexts(first:3)` used 1 point (two calls moved `used` from 1 to 3, one of them was a trivial call). A query for one PR with `contexts(first:100)` stays tiny.
- Secondary limits: at most 100 concurrent requests across REST and GraphQL, 900 points per minute REST, 2,000 points per minute GraphQL, 90 s CPU per 60 s (60 s GraphQL), about 80 content-creating requests per minute and 500 per hour. A GraphQL query costs 1 point, a mutation 5. Most REST GET 1, writes 5. [doc]
- Many PRs in one query: GraphQL aliases (`a: repository(...){pullRequest(number:1){...}}` `b: ...`) or `search(query:"is:pr is:open repo:o/r author:@me", type:ISSUE, first:100)`, as `git/pr/pr_list.go` already does. Cost scales with node count, so with `contexts(first:100)` x 100 PRs the predicted cost is about (100 + 100x100)/100 = 101 points. Poll the cheap fields (state, headRefOid, rollup `state`) for all PRs in one call, and fetch `contexts` only for a PR whose rollup state is `FAILURE`/`ERROR` or whose `headRefOid` changed. [doc for cost formula; the two-tier plan is a recommendation]
- Search API has its own limit: 30 requests per minute for authenticated search. [run: `search.limit 30`] But GraphQL `search(...)` uses the GraphQL budget, not this one. [open: not stated in the pages fetched; the `rate_limit` output showed `search` unchanged after GraphQL queries elsewhere, and gx's own code relies on GraphQL search.]
- Conditional requests (REST only): send `If-None-Match: <etag>`. `304 Not Modified` does not count against the primary limit if the request is authenticated. [doc]
  [run] Observed on `gh api repos/cli/cli/pulls/14976`: first response had an `Etag`; a repeat with `If-None-Match` returned `HTTP/2.0 304`, and `X-Ratelimit-Remaining` did not move. `gh api` exits 1 and prints `gh: HTTP 304` on a 304, so the server must read the status from `-i` output rather than the exit code.
  The ETag changes if the request parameters change, so keep requests stable. [doc]
  GraphQL has no ETag support documented. [open] Use REST + ETag for the cheap "did anything change" probe, GraphQL for the detail.
- `gh api --cache <duration>` caches responses locally. It is a fixed TTL, not an ETag check, so it is not useful for polling. [doc: `gh api --help`]
- `x-poll-interval` header: when present, wait at least that many seconds. [doc: best practices] It was not present on the PR response. [run]
- Webhooks are the documented first choice over polling. [doc] They need a public endpoint, which gx's local server does not have. So polling it is.
- Sensible interval (recommendation, not a doc number): 30 to 60 seconds while CI is `PENDING` on an open PR, 2 to 5 minutes for an open green or draft PR awaiting a merge, and back off to 5 to 10 minutes after a long idle. At 60 s a single-PR watcher costs 60 GraphQL points per hour out of 5,000 (1.2 percent). Ten PRs in one aliased query still cost about 60 points per hour.
- Back off on `x-ratelimit-remaining` reaching 0 until `x-ratelimit-reset`, and on a `retry-after` header. [doc] The headers are authoritative over `/rate_limit`. `GET /rate_limit` is free against the primary limit but can count against the secondary one. [doc]

## 6. Telling gh failures apart

Observed with `gh` 2.103.3 [run]:

| Case | How to detect | Evidence |
|---|---|---|
| gh not installed | `exec.LookPath("gh")` fails / `exec.ErrNotFound` from `cmd.Run` | `git/pr/pr_errors.go` `errors.Is(err, exec.ErrNotFound)` |
| not logged in | exit code **4**. stderr: `To get started with GitHub CLI, please run:  gh auth login` and `Alternatively, populate the GH_TOKEN environment variable ...` | exit 4 observed with an empty `GH_CONFIG_DIR`. [doc: `gh help exit-codes` "requires authentication"] |
| not logged in, via status | `gh auth status` exits 1 and prints `You are not logged into any GitHub hosts. To log in, run: gh auth login`. With `--json` it always exits 0, so avoid `--json` for this check. | observed. [doc: `gh auth status --help`] |
| expired or bad token | not tested here. `gh auth status` reports "authentication issues" and exits 1. [doc] Exit 4 with an HTTP 401 is likely but not documented. [open] |
| repo missing or no access | `gh pr view -R o/r` stderr: `GraphQL: Could not resolve to a Repository with the name 'o/r'. (repository)`, exit 1. REST: `gh: Not Found (HTTP 404)`, exit 1. | observed. GitHub answers 404 for a private repo the token cannot see, so "does not exist" and "no access" look the same. [doc: the REST error body links to docs; the 404-for-private behavior is standard GitHub, not re-verified on a fetched page] |
| not in a git repo (no `-R`) | `failed to run git: fatal: not a git repository`, exit 1 | observed |
| branch has no PR | `no pull requests found for branch "<b>"`, exit 1 | observed |
| PR number does not exist | `GraphQL: Could not resolve to a PullRequest with the number of N. (repository.pullRequest)`, exit 1 | observed |
| rate limit | HTTP 403 or 429 with `x-ratelimit-remaining: 0` or `retry-after`. [doc] gh wording not observed. [open] |
| 304 from `gh api -H If-None-Match` | stderr `gh: HTTP 304`, exit 1 | observed |

So exit 4 separates "not logged in" from everything else. Exit 1 is shared by all other failures, so those are told apart by stderr text, which is not a stable contract. [open: gh documents only the exit codes 0, 1, 2, 4.] Prefer calling `gh api` with `-i` and reading the HTTP status line for 401, 403, 404, 429, which is a documented contract of the REST API.

Server note: set `GH_TOKEN` is an alternative to a stored login. [doc: `gh help environment`] Also `GH_PROMPT_DISABLED`-style non-interactivity is not documented in the fetched help; since stdin is not a TTY under the server, gh does not prompt. [open] Pass `--title`/`--body` and `--head` to `pr create` so it cannot want to prompt.

## 7. What gx's `git/pr` already does

Files: `git/pr/pr.go`, `pr_list.go`, `pr_graphql.go`, `pr_errors.go`, `pr_closed.go`, `pr_comments.go`.

- `runGH(dir, args)`: runs `gh` with `cmd.Dir = dir`, captures stdout and stderr, and returns `*run.RunError{Args, Dir, Stdout, Stderr, Code}` on a non-zero exit, so exit code and stderr are kept. Reusable as is. A server version should add a context timeout and `-R` instead of relying on `cmd.Dir`.
- `runGraphQLRequest(dir, query, extraArgs, target)`: `gh api graphql -f query=... [-f/-F var=...]`, decodes JSON, and surfaces `errors[0].message`. Reusable for the 3c query with a new envelope type.
- `classifyPRListError` / `PRListError{Kind}`: `GHNotInstalled` via `exec.ErrNotFound`, `Unauthenticated` via stderr containing `gh auth login`, else `Generic`. Reusable. It does not use exit code 4 and does not separate "no access" or "rate limited". Add: `Code == 4` as unauthenticated, and kinds for 404/403/429.
- `PR.CIState()`, `MergeableState()`, `failedCheckConclusions`: roll-up logic over `status`/`conclusion` strings. Reusable for a quick roll-up, but fix `STARTUP_FAILURE` and decide `CANCELLED` (see 3c, 3e). The GraphQL path (`graphQLRollupToChecks`) collapses the roll-up to one synthetic check, so it loses per-check names. The server needs a new per-check query (3c) instead.
- `BranchPRURL` (`gh pr view --json url`), `CommitPRURL` (`gh pr list --search <hash> --state all`): the TUI uses them for "open PR from a commit". The server should look a PR up by number, stored on the epic, not by branch or by search. Search by commit hash will not find a squash-merged PR's trunk commit reliably. [open: not tested.]
- `IsCommitMergedToTrunk`: ancestry check. Wrong for squash and rebase merges (see section 2).
- `ListOpenPRs`: search `is:pr author:@me is:open repo:<r>`, `first: 100`, GraphQL. It is the model for the many-PR poll. It does not page (no `pageInfo`), and it selects `author:@me`, so a PR opened under another account is invisible.
- Not present in gx: any call that creates a PR, marks ready, reads `headRefOid`, `mergeCommit`, `state`, run ids or logs. All of these are new.

## 8. Not settled by the docs (summary)

1. Whether a rerun of an Actions job keeps its job id (`CheckRun.databaseId`). The recommended key avoids needing it.
2. Log size limits and log retention. Not on the REST workflow-runs, workflow-jobs or Actions limits pages.
3. Behavior of `gh run view --log-failed` on an in-progress run, and for an expired log.
4. Whether GraphQL supports conditional requests or ETags. Not documented.
5. `gh pr ready` on an already-ready PR.
6. Which exit code and text `gh` gives for an expired token and for a rate-limited call. Only exits 0, 1, 2, 4 and (for `pr checks`) 8 are documented.
7. Whether `workflowRun` is null for non-Actions checks. The schema says nullable. Not run on such a check.
8. Whether `CANCELLED` on the head commit comes from a superseded run or a person. (Treat as "do not fix" or ask.)
9. Which poll interval GitHub prefers. No `x-poll-interval` was sent on the PR endpoint, and the docs give no number.
