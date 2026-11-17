import { useEffect, useMemo, useState } from "react"
import type { FetchEpicDetail } from "../epic/detail"
import { useKeyedFetch } from "../useKeyedFetch"
import type { FetchDiffCommits, FetchDiffFiles } from "./api"
import { groupCommits } from "./commitGroups"
import type { DiffUrl } from "./diffUrl"

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
  nonce: number,
  fetchers: {
    fetchFiles: FetchDiffFiles
    fetchCommits: FetchDiffCommits
    fetchEpic: FetchEpicDetail
    fetchArchivedEpic: FetchEpicDetail
  },
) {
  const { fetchFiles, fetchCommits, fetchEpic, fetchArchivedEpic } = fetchers
  const { project, first, second } = url
  const canFetch = !!project && !!first && !!second
  const key = `${project}\n${first}\n${second}\n${nonce}`
  const titles = useTicketTitles(url.epic, fetchEpic, fetchArchivedEpic)

  const commitsFetch = useKeyedFetch(canFetch ? key : null, () =>
    fetchCommits(project, first, second),
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
  const narrowedFetch = useKeyedFetch(narrowedKey, () =>
    fetchFiles(project, first, second, narrowing),
  )
  const narrowedFiles =
    narrowedFetch && "value" in narrowedFetch && narrowedFetch.value.kind === "files"
      ? narrowedFetch.value.files.files
      : []

  const files = useKeyedFetch(canFetch ? key : null, () => fetchFiles(project, first, second))

  return { canFetch, files, returned, wanted, narrowing, emptyNarrowing, groups, narrowedFiles }
}
