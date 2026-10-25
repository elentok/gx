# United Epics page: data sources

Question (`gx:united-epics-page/01`): can `/v1/epics` (`server/epiclist.go`, `EpicListRow`) feed every
card of a single Epics page that replaces the Queue, Epics and Designs tabs? Or does the page also
need the Queue's live snapshot (`/v1/snapshot`)?

Short answer: no, `/v1/epics` alone is not enough. It has the state, the section inputs, the
progress, the merge-block reason, unmerged blocker epics and `impl_address`. It lacks pause and
deadlock reasons, per-ticket questions and approvals, the design session's live and question marks,
Designs that have no map yet, and one-off tickets. All of those are in the snapshot today. Every
page already holds the snapshot stream through one shared connection.

All paths are relative to the repo root. Line numbers are at commit `ceb63b32`.

## The two payloads

- `/v1/epics` returns `EpicList` (`server/epiclist.go:113-117`): a list of `EpicListRow`
  (`server/epiclist.go:69-103`) plus follow-up counts. It lists every epic except follow-ups,
  archived epics and one-off epics (`server/epiclist.go:121-123`). Rows are built per request in
  `epicList` (`server/epiclist.go:162-205`).
- `/v1/snapshot` returns `Snapshot` (`server/index.go:77-108`), built in `server.snapshot`
  (`server/server.go:687-714`). Its `epics` field is one `EpicSummary` per epic that holds a queue
  item (`server/epicsummary.go:83-114`). It skips map epics and merged epics
  (`server/epicsummary.go:104`). It also carries `pending` (`server/verdicts.go:33-38`), `mode`,
  `sessions` (`server/sessiontypes.go:10-44`), `recently_finished`
  (`server/recentlyfinished.go:15-24`) and every ticket (`server/index.go:19-72`).

## Fact by fact

| Fact | Source today | Gap |
| --- | --- | --- |
| State of a loop epic | Both. List: `epicListState` (`server/epiclist.go:128-146`) wraps `epicStateFor` (`server/epicsummary.go:256-286`) and adds queued, ready, draft, merge-blocked, no-open-tickets. Snapshot: `EpicSummary.State` from the same `epicStateFor` (`server/epicsummary.go:185`). | None for state. The list says `queued` where the snapshot says `waiting`. The Queue card's "Waiting: queue paused" badge needs `Snapshot.mode` (`webui/src/view.ts:64-69`), which the list lacks. |
| State of a Design | List only: `mapListState` gives designing, waiting-to-design or finished (`server/epiclist.go:150-160`). Snapshot has no map cards (`server/epicsummary.go:104`). | A Design whose map Chart has not created yet has no row. The Designs picker makes one from its session (`webui/src/design/view.ts:88-109`). |
| Section (Designs) | List: `picker_section` and `picker_sort_at` on map rows (`server/epiclist.go:100-101`, `server/mappicker.go:44-83`). | The new sections (In progress, Ready to queue, Ready to design, Finished) do not match `PickerSection` (`server/epiclist.go:57-62`) or the Epics tab sections (`webui/src/epics/view.ts:22-41`). A client can derive them from `state`, `queued` and `picker_section`. |
| Section (loop epics) | List: `state`, `queued`, `queueable` (`server/epiclist.go:74-85`). The Epics tab shows only ready, designing, waiting-to-design and finished in its sections. Other states appear only in search (`webui/src/epics/view.ts:22-41`). | None in the data. |
| Progress done/total | Both. List: `done`, `total` (`server/epiclist.go:259`). Snapshot: `EpicSummary.done/total` (`server/epicsummary.go:149`), zeroed for the one-offs root (`server/epicsummary.go:181-184`). | None. |
| gx-design step | List: `progress_step` from `design_step` (`server/epiclist.go:209`, `server/mapprogress.go:14-22`). Snapshot: `SessionInfo.reported_step` and `reported_title`, the step the agent reports now (`server/sessiontypes.go:37-41`). The per-step list is only in `/v1/epics/detail` (`server/epicdetail.go:115`, `server/mapprogress.go:38-55`). | The list has the stored step, not the reported live step. |
| Pause reason | Snapshot only: `EpicSummary.reason` and `more` (`server/epicsummary.go:68-73`, `186-187`), ticket `reason` and `reason_label` (`server/epicsummary.go:158-159`). | Missing in the list. A row can have state `paused` with no reason (`server/epiclist.go:138-139`). |
| Deadlock reason | Snapshot only: `reason` "Dependency deadlock" and `detail` from `cycleReason` (`server/epicsummary.go:192-193`). | Missing in the list. |
| Merge-block reason | Both. List: `land_block_kind`, `land_block_reason`, set only when state is `merge-blocked` (`server/epiclist.go:93-94`, `269-271`). Snapshot: `merge_blocked`, `merge_block_kind`, `detail` (`server/epicsummary.go:189-191`). | None. The JSON names differ (`land_block_*` and `merge_block_kind`). |
| Unmerged blocker epics | List only: `blocked_by_epics` (`server/epiclist.go:86-88`, `196`, `226-238`). | Missing in the snapshot. |
| Live ticket agent | List: state `running` (needs a server claim time, `server/epicsummary.go:273-281`). Snapshot: `active` tickets with `claimed_at` (`server/epicsummary.go:173-177`), listed only while running or merging (`server/epicsummary.go:196-198`). | The list says an epic runs, not which tickets. The new card does not list tickets, so this may not matter. |
| Live design session | Snapshot: `sessions` with `design` and `status.state` (`server/sessiontypes.go:18-20`). The picker marks a row live when a session of the map is not exited (`webui/src/design/view.ts:57-65`). The server computes the same fact in `designActivities` (`server/mappicker.go:22-40`) but uses it only to pick the section (`server/mappicker.go:54`). | Missing in the list. |
| Question on a Design | Snapshot: `SessionInfo.actionable.grilling` (`server/sessiontypes.go:72-84`), read by the picker (`webui/src/design/view.ts:62`). Failure: `SessionInfo.failure` (`webui/src/design/view.ts:63`). | Missing in the list. Non-grilling Actionables (`kind`, `reason`, `action`, `target`, e.g. the `queue` kind at `server/sessiontypes.go:86-94`) reach the snapshot, but no list page reads them. |
| Question on a loop ticket | Snapshot: paused ticket `summary`, `answerable`, `answered` (`server/epicsummary.go:160-172`), active ticket `needs_approval` (`server/epicsummary.go:108-110`). The Queue card links to the ticket (`webui/src/view.ts:75-100`). | Missing in the list. The snapshot has it only for epics that hold a queue item (`server/epicsummary.go:89`). |
| Queued or running one-off | Snapshot: the `one-offs` root gets an `EpicSummary` like any queued epic (`server/epicsummary.go:89-112`, `180-184`). `pending` lists each queued ticket and its verdict (`server/verdicts.go:192-212`). | Missing in the list: one-off epics are never listed (`server/epiclist.go:122`). |
| Finished one-off | Snapshot: `recently_finished` with `one_off` (`server/recentlyfinished.go:26-53`, `74-76`), capped by the `RecentlyFinished` setting (`server/server.go:705`). | Missing in the list. |
| Finished loop epic | Both. List: every finished epic with `merged_at` and `completed_at` (`server/epiclist.go:77-78`). Snapshot: the newest N only (`server/recentlyfinished.go:58-88`). | None. |
| Design to impl epic | List and detail: `impl_address`, found by the name `<map>-impl` (`server/mappicker.go:45-50`, `server/epicdetail.go:176-178`). | None for this direction. |
| Impl epic to Design | Nothing. The detail of a loop epic has no Design address (`server/epicdetail.go:191-199`). The epic page only checks the name suffix to show "Still publishing tickets" (`webui/src/epic/EpicPage.tsx:140`). | Missing. A client can strip `-impl` from the name. |

## How a Design and its impl epic find each other

The link is the name only, in both directions.

- `ImplEpicSuffix = "-impl"` (`tickets/epic_add.go:18-19`).
- Design to impl: the server looks up `<map>-impl` in the same project's epics
  (`server/mappicker.go:45-50`, `server/epicdetail.go:176-178`, `tickets/design_step.go:72`).
- Impl to Design: the tickets package strips the suffix and checks that the result is a map
  (`tickets/epic_add.go:195-205`, `tickets/map_done.go:17`). Neither `EpicListRow` nor `EpicDetail`
  carries the reverse address.
- No front-matter field stores the link. The row and the detail recompute it on every request.

## How each tab gets live updates today

All three tabs share one connection. `App` creates it and provides it through `ConnectionContext`
(`webui/src/App.tsx:9-15`), so the snapshot stream runs on every page.

- **Queue tab.** `createConnection` fetches `/v1/snapshot`, then opens SSE `/v1/events?since=<seq>`
  (`webui/src/connection.ts:153-197`). Every event marks the snapshot stale and triggers a refetch.
  Refetches run one at a time, with at most one queued (`webui/src/connection.ts:131-151`,
  `171-174`). A 45 s watchdog and a 1 s to 15 s backoff handle a dead link
  (`webui/src/connection.ts:5-8`, `117-129`). The snapshot is cached in `sessionStorage`
  (`webui/src/main.tsx:12`, `webui/src/snapshotCache.ts`). After a merge, the card calls
  `conn.refresh` (`webui/src/routes.tsx:21`).
- **Epics tab.** `useEpicList` uses `useFetched("epics")` (`webui/src/epics/list.ts:22-25`). It
  fetches `/v1/epics` on mount, and again 500 ms after events settle and on reconnect
  (`webui/src/useFetched.ts:18-46`, `webui/src/refetch.ts:4-27`). After a Queue click it calls
  `retry` (`webui/src/epics/EpicsPage.tsx:69`). There is no polling.
- **Designs tab.** The sidebar reads `/v1/epics` the same way and reads `sessions` from the
  snapshot (`webui/src/design/DesignPage.tsx:192-194`, `212`).
- The server emits events for ticket changes (`server/index.go:163-177`), queue changes
  (`server/queue.go:187`), session changes (`server/sessions.go:179`) and more. Every event of any
  type triggers both the snapshot refetch and the debounced `/v1/epics` refetch.

## What one payload has and the other lacks

Snapshot has, `/v1/epics` lacks:

- Queue mode (`server/index.go:94-96`), AFK, budget, build.
- Pause reason with `more`, deadlock reason and cycle detail, `has_repair`
  (`server/epicsummary.go:65-75`).
- Per-ticket lines: active tickets with `claimed_at` and `needs_approval`, paused tickets with
  `summary`, `answerable`, `answered`, `recovery` (`server/epicsummary.go:35-52`).
- Sessions: live state, Actionable, grilling, failure, auto-advance, reported step
  (`server/sessiontypes.go:10-44`).
- One-offs: the queued `one-offs` card, the `pending` verdicts, finished one-offs.

`/v1/epics` has, the snapshot lacks:

- Every epic that holds no queue item: ready, draft, no-open-tickets, maps, all finished epics.
- `queueable`, `queued`, `blocked_by_epics`, `created_at`, `queued_at`, `last_activity_at`,
  `merged_at` (`server/epiclist.go:77-90`).
- Map fields: `progress_step`, `picker_section`, `picker_sort_at`, `impl_address`
  (`server/epiclist.go:96-102`).
- Follow-up counts (`server/epiclist.go:105-117`).

## Gaps

1. `/v1/epics` has no pause reason and no deadlock reason.
2. `/v1/epics` has no per-ticket question, answered or needs-approval flags for loop epics.
3. `/v1/epics` has no live design session, Design question or Design failure flag. The server
   computes live and waiting in `designActivities` but does not send them.
4. A Design without a map yet has no `/v1/epics` row. Today the client builds one from the session.
5. One-offs, queued or finished, are not in `/v1/epics`.
6. No field points from an impl epic to its Design. The link is the `-impl` name only.
7. The queue mode, needed for a "queue paused" badge, is only in the snapshot.
8. The new page's four sections match neither `picker_section` nor the Epics tab sections. The
   client must derive them.
