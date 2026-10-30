# Skill directory installation for Claude and Codex

Status: implemented and verified on 2026-10-30.

## Problem and evidence

The installer currently manages each bundled file separately. In dev mode it
creates links for `SKILL.md` and supporting files. If an installed skill directory
already links into the checkout, replacing a file through that directory can
replace the source file with a link to itself. Uninstall has the same directory
traversal risk because it removes manifest-recorded file paths.

The isolated regression in `skills/install_alias_test.go` reproduced source
damage. The directory-aware installer replaces that temporary traversal guard
and protects installation, migration, forced replacement and uninstall.

Relevant implementation:

- `skills/roots.go`: Codex targets `.agents/skills`.
- `skills/install_plan.go`, `skills/paths.go`: entries, ownership and physical root aliases.
- `skills/transaction.go`: backups, serialized writes, rollback and manifest publication.
- `skills/uninstall.go`: removal of individually recorded paths.
- `skills/manifest.go`, `skills/ownership.go`, `skills/report.go`: schema and ownership rules.
- `cmd/skills.go`: explicit install and automatic startup refresh.

The inspected manifest uses schema v1, records 67 files, and remembers dev mode
and this checkout. On this machine `.agents/skills` links to `.claude/skills`,
which links to the dotfiles skill directory. Both discovery roots therefore
address the same physical installation. These are observations, not defaults
to hard-code or live state to use in tests.

## Agreed behavior

### Layout and discovery

- Claude uses `~/.claude/skills`; Codex uses `~/.agents/skills`.
- Dev mode creates one directory link per bundled skill, pointing to its checkout
  directory. `SKILL.md` remains a real file inside that source directory.
- Ordinary installs place bundled copies in real skill directories and require
  no checkout after installation.
- Shared files `gx.md`, `gx-local-tracker.md` and `gx-failing-tests.md` retain their
  sibling layout beside skill directories. Dev installs link these individual
  files; ordinary installs copy them.
- Directory links expose their source directory contents. Dev installation no
  longer selects individual embedded files inside each linked skill directory.
- Preserve user-managed discovery-root links. Resolve root aliases and perform
  each physical write or deletion only once.
- Preserve relative references to shared files and sibling skills in both modes.

[Codex documentation](https://learn.chatgpt.com/docs/build-skills) documents the
`.agents/skills` root and directory-link support. The user's observed failure of
file-linked `SKILL.md` is an acceptance requirement. Confirm actual discovery
with an isolated Codex check rather than relying on filesystem shape alone.

### Ownership, updates and uninstall

- Introduce a versioned manifest representation that distinguishes managed skill
  directory links, standalone file links and copied files. Keep v1 readable for
  migration. Never reinterpret v1 file entries as directory ownership.
- Record the link target for directory links. A correct unchanged link needs no
  filesystem replacement on repeat installation.
- Updating or uninstalling a directory-linked skill operates on the directory
  link itself. Never traverse its contents to update, back up or delete them.
- Keep ownership of copied files explicit. An installation does not own every
  file inside a real directory merely because it installed a skill there.
- Plan cleanup for removed bundle entries and retired roots. Do not discard
  ownership records while leaving old managed content unaccounted for.
- Preserve source checkout files under every install, update, force, migration,
  uninstall and rollback path. Force never permits writing through a skill link.
- Switching copy/dev modes or retargeting a different checkout requires explicit
  force, including when the current installation is unchanged.
- A forced replacement backs up the replaced content. Unrelated content outside
  the explicitly forced replacement remains untouched.

### Migration and conflicts

- Automatically migrate verified, unchanged legacy installations without changing
  their existing mode or dev checkout.
- Adopt an existing skill directory link when its target is exactly the requested
  source directory, including when v1 only records individual files. Record the
  directory link without changing it or its target contents.
- Recognize legacy file links and managed copies through their recorded identity.
  Local edits, wrong link targets and unrelated content cause a conflict unless
  the person explicitly forces the affected replacement.
- After the new installation succeeds, remove unchanged gx-owned entries from
  `.codex/prompts`. Back them up first. Preserve unrelated prompts; local edits to
  owned prompts block automatic migration.
- Apply old-root cleanup to physical paths so aliases cannot cause deletion of
  content retained in the new installation.

### Backups and failed operations

- Build the complete operation plan and check all destinations before mutating
  any installation. Coordinate install, refresh and uninstall so they cannot
  concurrently change the same installation.
- Keep dated backups under gx's data directory and print the backup location.
  Retain backups until the person explicitly removes them.
- Preserve symlinks as symlinks in backups. Do not copy their target directories.
- If a migration write fails, restore the previous installation from the saved
  state. Publish the new manifest only after all writes and cleanup succeed.
- If restoration itself fails, report the affected paths and backup location.
  Do not report a successful migration or publish a manifest claiming success.

### CLI and server startup

- Use the same ownership, planning and migration behavior for manual install and
  startup refresh. Keep filesystem policy in the skills package.
- Startup refresh remembers the recorded mode and checkout. It never forces a
  conflicting replacement or silently switches to a different checkout.
- A startup migration conflict leaves the installation unchanged. The server
  still starts and shows a persistent warning naming the conflicting path and
  directing the person to `gx skills install`.
- Update help and bundled layout documentation for directory links, the Codex
  discovery root, migration, force behavior and backup recovery.

## Implementation sequence

- [x] Add the manifest entry representation and v1 migration reader, with ownership
  and root-alias classification tests.
- [x] Implement directory-aware planning, placement and uninstall. Keep standalone
  shared files and copied-file ownership explicit.
- [x] Implement migration, retired-entry cleanup, backups and rollback through the
  same installation module. Rework the exploratory guard and regression test.
- [x] Connect both discovery roots, CLI reporting and startup refresh behavior.
- [x] Update documentation and complete the verification below.

## Verification

- [x] Dev install exposes skills through directory links and leaves source files
  as ordinary files. Ordinary install exposes copied skills without a checkout.
- [x] Repeat installs are unchanged; different checkout and mode changes require
  force. Forced replacements retain recoverable backups.
- [x] Both distinct roots and aliased roots work. Aliased roots receive no duplicate
  writes or deletes. Preserve the user's discovery-root symlink chain.
- [x] Migrate v1 file links, managed copies and existing correct directory links.
  Cover modified files, unrelated content inside real directories, wrong targets,
  stale entries, old prompt roots and partially missing legacy files.
- [x] Uninstall removes owned directory links while preserving the source tree;
  copied installations preserve unrelated files. Repeat uninstall is safe.
- [x] Inject failure after an earlier write succeeds. Verify rollback, unchanged
  previous manifest and usable retained backups. Verify rollback-failure reporting.
- [x] Compare checkout paths, file types and contents before and after lifecycle
  operations. Catch self-links, source deletion and writes through linked parents.
- [x] Startup refresh migrates unchanged installs, preserves dev source selection,
  never forces conflicts, and keeps the server usable with a persistent warning.
- [x] Run isolated Codex discovery with temporary home/config roots, using a
  directory-linked skill and its supporting files. Verify shared sibling references
  resolve from the reported skill path. Do not read the user's real agent config.
- [x] Run the skills package tests and targeted CLI/server integration checks.
  Run repository-required changed-file verification; avoid an unrelated full suite.

## Verification results

- The complete skills and environment-guard packages passed: 194 tests.
- Targeted skills, CLI, server and environment checks passed: 107 tests, including
  automatic v1 migration, persistent conflict warnings and isolated Codex discovery.
- Codex CLI 0.166.0 discovered the installed directory link through a temporary
  `.agents/skills` root. Its advertised skill path resolved supporting files and
  the shared sibling file. The check used no real agent configuration or model turn.
- Changed-file formatting and whitespace checks passed.
- A full CLI package run failed only at the existing
  `TestRunEditorCommand_ChildGetsTheGivenEnv`. This machine's `/usr/bin/printenv`
  returns success when asked for a present variable and a missing variable. That
  behavior was reproduced separately with an empty environment. The editor code
  and test are unchanged.

Implementation and commit were subsequently requested by the user. Live skill
installations were not repaired or migrated during this work. No push was requested.
