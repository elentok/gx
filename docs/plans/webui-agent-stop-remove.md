# Web UI: stop and remove agents, auto-close finished designs

Branch signal check: confirmed. `design_step` reaching `done` is written to the map epic and noticed by
`scanDesignCleanups` (server/designs.go). Today it only cleans the worktree; the session stays live.
`branch` is not used: the agent still runs the last step then.

## 1. Stop and Remove in the web UI

- [x] Server: web `stop` accepts any non-Queue session (was design-only), token still required
- [x] Server: web `POST /v1/sessions/remove`, same check; Queue sessions refused (`queue-session`)
- [x] Regenerate `webui/src/generated/server.ts`
- [x] UI: "Stop" and "Remove" buttons in the agent header (`SessionView`), Remove confirms
- [x] UI: Remove on a live agent stops it first, then removes, then leaves the page

## 3. Bulk clean-up

- [x] UI: "Remove exited design agents (N)" button next to the Design agents switch in the sidebar

## 2. Auto-close finished designs

- [x] Server: when `scanDesignCleanups` sees `design_step` move to `done`, stop the map's live design session
