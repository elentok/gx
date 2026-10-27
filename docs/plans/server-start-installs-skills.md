# Server start installs skills

Ticket: gx:skills-sync-install/04.

`gx server start` and `gx server restart` install gx's skill bundle before they start the server.

- No manifest: install the embedded bundle as managed copies.
- Managed-copy manifest: install the embedded bundle as managed copies.
- Symlink (dev) manifest: re-run the dev install from the checkout named in the manifest. If that
  checkout is gone, warn and leave the skills alone.
- A conflict or failure prints a warning and the server still starts.
- The CLI writes the problem (or nothing) to `skills-install.json` in the state dir. The server
  puts it in the snapshot, and the web UI shows it as a toast that stays until closed.

## Tasks

- [x] `skills`: dev source prefix helper + install-status file read/write
- [x] `cmd`: `refreshSkills` (pick mode from manifest, install, write status)
- [x] `cmd`: call it from `gx server start` (restart goes through start)
- [x] `server`: `skills_install_problem` in the snapshot
- [x] regenerate `webui/src/generated/server.ts`
- [x] web UI: persistent toast with a close button
- [x] tests (cmd, server, web UI)
- [x] update `skills/gx.md` / README install docs
- [x] verify
