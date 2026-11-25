import { useCallback, useEffect, useMemo, useState } from "react"
import type { FetchEpicDetail } from "../epic/detail"
import { useLatest } from "../hooks"
import { useKeyedFetch } from "../useKeyedFetch"
import type { FetchDiffCommits, FetchDiffFiles, FetchDiffWorktree } from "./api"
import { groupCommits } from "./commitGroups"
import { type DiffUrl, worktreeValue } from "./diffUrl"

/** Ticket titles by address: the epic's rows, or its archived detail when the epic is archived. */
function useTicketTitles(
  epic: string | undefined,
  fetchEpic: FetchEpicDetail,
  fetchArchived: FetchEpicDetail,
) {
  const [titles, setTitles] = useState<ReadonlyMap<string, string>>(new Map())
  useEffect(() => {
    if (!epic) return
    let current = true
    fetchEpic(epic)
      .then((r) => (r.kind === "ok" ? r : fetchArchived(epic)))
      .then((r) => {
        if (current && r.kind === "ok")
          setTitles(new Map(r.detail.rows.map((x) => [x.address, x.title])))
      })
      .catch(() => {})
    return () => {
      current = false
    }
  }, [epic, fetchEpic, fetchArchived])
  return titles
}

/** The data side of the Diff page: the range's files and commits, and the files narrowed to chosen commits. */
export function useDiffData(
  url: DiffUrl,
  fetchers: {
    fetchFiles: FetchDiffFiles
    fetchCommits: FetchDiffCommits
    fetchWorktree: FetchDiffWorktree
    fetchEpic: FetchEpicDetail
    fetchArchivedEpic: FetchEpicDetail
  },
) {
  const { fetchFiles, fetchCommits, fetchWorktree, fetchEpic, fetchArchivedEpic } = fetchers
  const { project, worktree } = url
  const titles = useTicketTitles(url.epic, fetchEpic, fetchArchivedEpic)

  // Only the focus event and an explicit refetch ask again; the old answer stays on screen meanwhile.
  const [tick, setTick] = useState(0)
  const refetch = useCallback(() => setTick((n) => n + 1), [])
  const refetchRef = useLatest(refetch)

  // Worktree mode: the route's answer holds the SHAs, so it comes first. Each ask snapshots the
  // worktree and gives new SHAs, which are new keys for everything below.
  const target = worktree ? worktreeValue(worktree) : ""
  const worktreeFetch = useKeyedFetch(
    project && target ? `worktree\n${project}\n${target}` : null,
    () => fetchWorktree(project, target),
    tick,
  )
  const answer = worktreeFetch && "value" in worktreeFetch ? worktreeFetch.value : undefined
  const noWorktree = !!answer?.reason
  const first = worktree ? (noWorktree ? "" : (answer?.base_sha ?? "")) : url.first
  const second = worktree ? (noWorktree ? "" : (answer?.snapshot_sha ?? "")) : url.second
  // The commit list reads base to HEAD; the snapshot is not a commit of the branch.
  const commitsTip = worktree ? (answer?.head_sha ?? "") : second
  const canFetch = !!project && !!first && !!second
  const key = `${project}\n${first}\n${second}`
  // A worktree refetch is the worktree call above; the SHAs it returns are the new keys.
  const refresh = worktree ? 0 : tick
  const canRefetch = canFetch || !!worktree
  useEffect(() => {
    if (!canRefetch) return
    const onFocus = () => refetchRef.current()
    window.addEventListener("focus", onFocus)
    return () => window.removeEventListener("focus", onFocus)
  }, [canRefetch])

  const commitsFetch = useKeyedFetch(
    canFetch ? `${key}\n${commitsTip}` : null,
    () => fetchCommits(project, first, commitsTip),
    refresh,
  )
  // Only commits the commit call returned can be narrowed to. Until it answers, the URL's SHAs
  // are held back so the file call is not made with a set that may be dropped.
  const returned = commitsFetch && "value" in commitsFetch ? commitsFetch.value : undefined
  const commitsPending = !!(url.commits || url.ticket) && !commitsFetch
  // A `ticket` key expands to the SHAs of the returned commits that carry that address.
  // `url.commits` is a fresh array on every parse, so it is rebuilt from its content to keep one identity.
  const commitsKey = url.commits?.join(",")
  const urlCommits = useMemo(
    () => (commitsKey === undefined ? undefined : commitsKey === "" ? [] : commitsKey.split(",")),
    [commitsKey],
  )
  const wanted = useMemo(
    () =>
      urlCommits ??
      (url.ticket
        ? (returned?.commits ?? []).filter((c) => c.ticket === url.ticket).map((c) => c.sha)
        : undefined),
    [returned, urlCommits, url.ticket],
  )
  const narrowing = useMemo(
    () => (returned ? wanted?.filter((s) => returned.commits.some((c) => c.sha === s)) : wanted),
    [returned, wanted],
  )
  const emptyNarrowing = !!wanted && !commitsPending && narrowing?.length === 0
  const groups = useMemo(
    () => (returned ? groupCommits(returned.commits, url.epic, titles) : []),
    [returned, url.epic, titles],
  )

  const narrowedKey =
    canFetch && narrowing?.length && !commitsPending ? `${key}\n${narrowing.join(",")}` : null
  const narrowedFetch = useKeyedFetch(
    narrowedKey,
    () => fetchFiles(project, first, second, narrowing),
    refresh,
  )
  const narrowedFiles =
    narrowedFetch && "value" in narrowedFetch && narrowedFetch.value.kind === "files"
      ? narrowedFetch.value.files.files
      : []

  const files = useKeyedFetch(
    canFetch ? key : null,
    () => fetchFiles(project, first, second),
    refresh,
  )

  return {
    canFetch,
    worktreeFetch,
    noWorktree,
    answer,
    refetch,
    files,
    returned,
    wanted,
    narrowing,
    emptyNarrowing,
    groups,
    narrowedFiles,
  }
}
