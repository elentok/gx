# Skills manager

Handoff spec from the `gx:skills-manager` map. Terms: **Skill source**, **Third-party skill**,
**Bundled skill** and **Vendored skill**, as in `GLOSSARY.md`.

## Problem Statement

A person keeps their own skills in one folder (`~/.dotfiles/core/ai/skills`) and makes
`~/.claude/skills` and `~/.codex/skills` symlinks to it. gx also installs its bundle into
`~/.claude/skills` and `~/.agents/skills`. With a skill root that is a symlink into the person's own
folder, the gx install either refuses (the destination overlaps a source) or writes gx's bundle into
the person's dotfiles. gx has no way to install the person's own skills for them. The person wants
`~/.claude/skills` to be a normal folder, holding gx's bundle and links to their own skills, kept
current without hand work.

## Solution

`gx skills install <dir>` makes the folder `<dir>` a **Skill source**. gx remembers it. For each
direct subfolder of the source that has a `SKILL.md`, gx puts a **Third-party skill** into both
discovery roots, `~/.claude/skills` and `~/.agents/skills`. By default each skill is one directory
link to the skill's folder, so the link covers the whole skill and never only its `SKILL.md`.
`--copy` installs managed copies instead, the same mode the gx bundle uses.

Rerunning the command is safe. A bare `gx skills install` and every `gx server start` re-sync the
bundle and every remembered source: a new skill folder gets links, and a link to a skill that left
the source is removed. A folder that gx did not install is never touched. The gx bundle stays as
managed copies, because it is embedded in the binary and cannot be linked.

`gx skills uninstall <dir>` removes one source's links and its record. `gx skills list` shows each
remembered source with its mode, roots and skill count.

When a skill root resolves into the source folder (the person's current setup), gx refuses and
prints the steps that make the root a real folder, so the person converts it by hand once. gx does
not convert a root itself.

## User Stories

1. As a person with a folder of my own skills, I want `gx skills install <dir>` to install each
   skill into `~/.claude/skills` and `~/.agents/skills`, so that both agents see my skills without
   me linking the whole folder.
2. As that person, I want each skill installed as one link to the skill's folder, so that the
   skill's reference files and scripts come along and my edits show up at once.
3. As that person, I want a folder without a `SKILL.md` and any loose file in the source to be left
   alone, so that notes and shared files in my folder are not published as skills.
4. As that person, I want `--copy` to install managed copies instead of links, so that I can use a
   source on a machine where links into the source are not wanted.
5. As that person, I want rerunning `gx skills install <dir>` to change nothing when nothing
   changed, so that I can run it any time.
6. As that person, I want a new skill folder in the source to get links on the next sync, so that I
   do not rerun anything by hand when I add a skill.
7. As that person, I want the links to a deleted or renamed skill folder removed on the next sync,
   so that no dangling skill shows up in my agents.
8. As that person, I want a bare `gx skills install` to re-sync the gx bundle and every remembered
   source, so that one command brings everything current.
9. As that person, I want `gx server start` to re-sync every remembered source, so that skills are
   current after a reboot without a manual step.
10. As that person, I want a bare install to use each source's remembered mode, so that a `--copy`
    source stays a copy source.
11. As that person, I want rerunning `gx skills install <dir>` with or without `--copy` to switch
    that source's mode without `--force`, so that I can change my mind without uninstalling first.
    Entries I changed after gx made them stay conflicts.
12. As that person, I want gx to treat the same folder reached through a symlink or a different
    spelling as the same source, so that I never get two records for one folder.
13. As that person, I want a skill name that two sources both have to be skipped in both sources,
    links and copies alike, with an error that names both sources, so that gx never picks a winner
    for me. The other skills of both sources still install, and a link gx already made for that name
    is removed.
14. As that person, I want a skill in my source with the name of a gx bundle skill to be skipped, and
    the bundle skill to install, with an error that names both owners and says to delete the old
    copy, so that a forgotten `gx-*` copy never removes gx's own skills.
15. As that person on first install, I want a link that already points at the skill's folder adopted
    as gx's own, so that my earlier hand-made links do not block the install.
16. As that person on first install, I want dangling links that point into the source folder
    removed, so that skills I deleted long ago stop showing up.
17. As that person, I want every other entry in a skill root left alone on first install, including
    ones with a name that matches a skill, so that gx never deletes anything it did not make or
    adopt.
18. As that person, I want gx to leave entries it did not record untouched (for example Claude's
    own `synced/` folder), so that other tools keep working in the same root.
19. As that person whose `~/.claude/skills` is still a symlink into my source folder, I want gx to
    refuse with steps to turn the root into a real folder, including deleting old `gx-*` bundle
    copies in the source, so that I can fix it once and not lose files.
20. As that person, I want gx never to write into my source folder through a root link, so that my
    dotfiles are never changed by an install.
21. As that person, I want a source folder that is gone to leave its links alone and report a
    problem that names `gx skills uninstall <dir>`, so that a drive that is not mounted does not
    wipe my skills.
22. As that person, I want `gx skills uninstall <dir>` to remove that source's links and its record,
    so that I can stop using a source cleanly.
23. As that person, I want `gx skills uninstall <dir>` to keep an entry that I changed after gx
    made it, unless I force it, as the bundle uninstall does, so that I do not lose edits.
24. As that person, I want a bare `gx skills uninstall` to still remove only the gx bundle, so that
    existing scripts keep their meaning.
25. As that person, I want `gx skills list` to show the bundle (or the dev checkout) first and then
    each source, each with its mode, roots and skill count, so that I can see what gx manages.
26. As that person, I want `--force` and `--force-all` to work for a third-party install as they do
    for the bundle, so that I can override a conflict I understand. They have no effect on a
    name clash, which is always skipped, never forced.
27. As that person, I want a conflict with an entry gx does not own (for example a real folder with a
    skill's name) to abort that source's install and name the conflicted paths, so that I never end
    with a partly installed source. A name clash between owners is not such a conflict: story 13 and
    14 skip the clashing skill and install the rest.
28. As that person, I want a problem with a source at server start to appear in the same skills
    problem the web UI already shows, so that I see it without reading a log.
29. As that person, I want a clean manual install or re-sync to clear that problem, so that the web
    UI stops showing a fixed problem.
30. As that person, I want a problem with one source not to stop the bundle or other sources from
    syncing, in a bare install as well as at server start, so that one broken folder does not block
    the rest. A bare install prints every problem and exits non-zero if any source failed or is gone.
31. As that person, I want the `$CODEX_HOME/skills` duplicate warning to cover my third-party skills
    too, so that Codex does not find a skill twice without my knowing.
32. As a gx developer, I want `gx skills install --dev` to behave as it does today, so that my
    checkout's bundle links are not mistaken for a third-party source.
33. As that person, I want `gx skills install <dir>` to fail with a clear message when `<dir>` does
    not exist or is not a folder, so that a typo does not create a record.
34. As that person, I want a source with no skill folders in it to install nothing and say so, so
    that I know the folder was read.
35. As that person, I want a skill that a later gx release (or a `--dev` checkout) adds to the bundle
    with a name one of my sources already owns to win, so that gx's own skills always install. gx
    removes my source's link for that name, skips that skill from then on, and reports the error
    from story 14. My other skills stay.

## Implementation Decisions

- **Domain.** The work is in the `skills` domain (manifest, ownership, install plan, transaction)
  and the `gx skills` command package. The manifest, ownership rules, install plan and transaction
  are reused. No ADR covers this area and none needs to change.
- **One record per source.** Each source has its own manifest, as the bundle has, keyed by an id
  derived from the source's real path (the folder after following symlinks). The implementer picks
  the id scheme and must keep it stable and safe as a file name. The manifest's source field holds
  the real path and a marker that tells a third-party source apart from the bundle and a dev
  install (the implementer picks the marker). Because the bundle has a manifest of its own, the
  "remembered sources" are the third-party manifests found in the manifest folder.
- **Source identity.** The real path after following symlinks. The same folder reached by two
  spellings or through a link is one source. Rerunning with or without `--copy` updates that
  source's record and mode.
- **What a skill is.** A direct subfolder of the source with a `SKILL.md`. A subfolder that is a
  symlink to a folder counts as that folder; the link in the root points at the subfolder's path
  inside the source. Nothing else in the source is installed. Loose top-level files stay out. The
  bundle keeps its loose top-level files (`gx.md` and the others) as today.
- **Roots.** Third-party skills go to the bundle's two roots, `~/.claude/skills` and
  `~/.agents/skills`, resolved the way the bundle resolves them. A root that does not exist is
  created.
- **Link mode (default).** One directory link per skill, the existing directory-link entry kind
  that `--dev` already uses. The link target is the skill's folder. The manifest owns the link,
  never the files behind it. A `SKILL.md` that is a symlink is still fine, because the link covers
  the whole folder and nothing is checked file by file.
- **Copy mode (`--copy`).** The same managed-copy mode as the bundle: file copies with hashes, the
  same conflict, force and backup rules. A copy install over an existing link of the same source
  replaces it as one entry, and a link install over copies does the same. The existing plan marks
  every entry as a conflict on a mode change (`skills/install_plan.go`, the mode-change check and
  the link-to-copy block). Both checks change the same way: an explicit `<dir>` rerun whose mode
  differs from the record counts as consent, and an entry gx owns unchanged switches without
  `--force`. Unchanged means a link that still points where gx recorded it, or a copy whose files
  are all intact with no extra file. A modified entry stays a conflict, and a conflict aborts the
  whole source's switch: no entry switches and the record keeps its old mode until
  `--force <path>` completes it. A bare install never switches a mode.
- **Entry removal.** A manifest entry whose skill left the source is removed on sync, the existing
  removal of entries a source no longer has. A link is removed only when it still points where gx
  recorded it. A link that was changed by the person is a conflict, not a removal.
- **Name clash.** Before any write, gx compares the skill names of every source, and of the embedded
  bundle (the skills built into the binary, whether or not the bundle is installed), or of the dev
  checkout in a dev install.
  - Two sources with the same skill name: that name is skipped in both. Each source still installs
    its other skills. A link or copy gx already made for that name from either source is removed
    as an entry the source no longer wants, under the usual ownership rules. The result does not
    depend on which source was installed first.
  - A source and the bundle (or the dev checkout) with the same name: the bundle (or dev) skill
    wins and installs. The source's skill is skipped, and an existing entry of the source for that
    name is removed first. This holds when a later gx release or a dev checkout adds the name.
    A source's own old `gx-*` copies fall under this rule.
  - Every skipped name is a skills problem. It names both owners, and for a bundle clash it tells
    the person to delete the old copy. Install exits non-zero. Server start records the problem.
  - `--force` and `--force-all` have no effect on a name clash.
  - A clash is not a conflict with an entry gx does not own: that case still aborts the source's
    install as in the first-install rule.
- **First install (no record for the source).** Per root, a link that already points at the same
  skill folder is adopted into the record. A dangling link that points into the source folder is
  removed. Everything else, including an entry with a skill's name that points elsewhere or is a
  real folder, is a conflict handled by the existing ownership and force rules. gx never touches an
  entry it did not record or adopt.
- **Overlap guard.** A skill root that resolves into the source folder is refused before any write.
  The existing destination and source overlap guard is extended to third-party sources and to every
  other source's recorded links. The message prints the steps: remove the root link, make the root a
  real folder, delete the old `gx-*` bundle copies in the source, then rerun. The refusal is a
  plain error from install and a problem at server start.
- **Source gone.** On a re-sync where the source folder no longer exists, gx changes nothing and
  reports a problem that names `gx skills uninstall <dir>`, as the dev checkout case does today.
- **Commands.**
  - `gx skills install <dir>` installs or re-syncs that one source. `--copy` is valid only with a
    `<dir>`. `--dev` and `<dir>` together are refused. A `<dir>` that is missing or not a folder is
    refused.
  - A bare `gx skills install` installs the bundle (or the dev checkout) as today, then re-syncs
    every remembered source in its recorded mode. Each source is synced on its own, as `Refresh`
    does: a conflict or a gone folder in one source does not stop the bundle or the other sources.
    It prints every problem, writes the same joined skills problem as `Refresh` (cleared when the
    run is clean), and exits non-zero if any install failed, any name was skipped or any source is
    gone. `gx skills install <dir>` and `--dev` never write or clear that problem for other
    sources: a clean install of one source cannot hide a problem in another.
    `--force` and `--force-all` apply to every path of every install in that run.
  - `gx skills uninstall <dir>` removes the links or copies of that source and then its record. The
    modified-entry and `--force` behaviour is the bundle uninstall's. The source need not exist.
    A bare `gx skills uninstall` removes only the bundle, as today.
  - `gx skills list` prints the bundle first (or the dev checkout, when a dev install is recorded)
    with its mode and roots, then one block or line per source: the real path, its mode, its roots
    and its skill count. With no bundle or dev install recorded, the first line says the bundle is
    not installed. With no sources it says so after that line.
  - Command help and the `gx skills` short description change to cover third-party sources.
- **Server start.** `Refresh` re-syncs the bundle and then every remembered source. It never forces
  and never fails its caller. Each source is synced on its own: one source's problem does not stop
  the others. All problems join into the one skills problem the server shows today, and a fully
  clean run clears it. A bare install that succeeds clears it too.
- **Codex duplicates.** The `$CODEX_HOME/skills` duplicate warning also runs over each third-party
  skill's name, with the same wording and the same rule: informational, never deletes, never
  recorded as a problem.
- **Dev mode.** `--dev` and its refresh behaviour do not change, with one addition: the dev install and
  dev refresh follow the Name clash rule, so a skill that the checkout adds and a source already owns
  makes the dev skill win and the source's skill skipped. gx's own checkout is not a third-party
  source.
- **Reports.** Install and uninstall print one line per target, as now, with the status words the
  bundle uses. A refused install prints the whole conflict set. A source with skills in it prints
  its skill count in the summary line.
- **Entries gx does not own.** Any entry in a skill root without a record is invisible to sync and
  uninstall. This includes Claude's `synced/` folder.
- **Person's migration.** gx does not convert `~/.claude/skills` or `~/.codex/skills`. The person
  does it once by hand with the printed steps. The dotfiles install script is not part of this work.

## Testing Decisions

- **Seam.** One seam: the `gx skills` command package, driven through the cobra command with fake
  dependencies pointing at temp directories (agent roots, manifest folder, status path, `$CODEX_HOME`
  and the source folder). `Refresh` is the same seam through its own entry point. Behaviour is
  checked on the filesystem (links, copies, manifests) and on the printed output and the recorded
  problem. This is the seam the bundle and dev tests already use, so no new seam is added. The
  domain package's own tests stay where they are and only grow for logic that the command seam
  cannot reach (name-clash detection across manifests, adoption on first install).
- **A good test** installs a real source folder into temp roots, runs the command, and asserts
  what a person would see: which entries exist in each root, where links point, what the output
  says, what the problem file holds. It does not assert manifest internals except where the manifest
  is the contract (one record per source, remembered mode).
- **Cases to cover**, mapped to the stories: link install of two skills into both roots, link
  points at the folder (2); loose files and folders without `SKILL.md` skipped (3); `--copy` (4);
  rerun is a no-op (5); new skill appears and removed skill's links go away (6, 7); bare install
  re-syncs every source in its recorded mode (8, 10); mode switch (11); same folder through a
  symlink is one source (12); name clash: two sources with one name skipping both, a source skill with a bundle name skipped while the bundle's installs, an existing link removed on a new clash, and a bundle or dev skill added later winning over a source's link (13, 14, 35); first-install
  adoption, dangling-link removal and untouched foreign entries (15, 16, 17, 18); a root that
  resolves into the source is refused with the steps and writes nothing (19, 20); source gone leaves
  links and names `uninstall <dir>` (21); `uninstall <dir>`, with a modified entry kept and then
  forced (22, 23); bare uninstall removes only the bundle (24); `list` output (25); `--force` and
  `--force-all`, which never override a name clash (26); a conflict with an unowned entry aborts that source's install (27); a bare install keeps going past a broken source and exits non-zero (30); `Refresh` with sources records a joined
  problem, keeps going past a broken source, clears on success (28, 29, 30); Codex duplicate
  warning on a third-party skill (31); `--dev` unchanged (32); bad `<dir>` and empty source (33, 34).
- **Prior art.** `cmd/skills/skills_test.go` (install, uninstall, dev, force, duplicate warning) and
  `cmd/skills/skills_refresh_test.go` (refresh, problem record, missing dev checkout). The domain
  tests for directory-link lifecycle and the overlap guard in `skills/` are the model for the
  domain-level additions.
- **Real filesystem.** Tests use real temp directories and real symlinks, never a faked filesystem,
  as the existing skills tests do. Run them capped, with the repo's process cap wrapper.

## Verification

Run against a temporary gx server with a throwaway home directory, a throwaway
`$CODEX_HOME`, and a source folder built for the check: two skill folders with `SKILL.md`, one
folder without, and one loose file.

1. Story 1: `gx skills install <src>`; exit 0. `~/.claude/skills/<skill>` and
   `~/.agents/skills/<skill>` exist for both skills and are directory links. No entry for the
   folder without `SKILL.md` or the loose file.
2. Story 2: `readlink` of each link is the skill's folder; a nested reference file in the skill is
   readable through the link in both roots.
3. Story 3: the folder without `SKILL.md` and the loose file are absent from both roots.
4. Story 4: `gx skills install --copy <src2>` for a second source gives real folders with copied
   files, not links; exit 0.
5. Story 5: rerun `gx skills install <src>`; exit 0, every target reads as skipped or the summary
   says up to date, and the link targets are unchanged.
6. Story 6: add a third skill folder to `<src>`, run bare `gx skills install`; its links appear in
   both roots.
7. Story 7: delete one skill folder, run bare `gx skills install`; its links are gone from both
   roots and the others stay.
8. Story 8: change a file in the source and in the other source, run bare `gx skills install`; the
   bundle and both sources are current in one run.
9. Story 9: add a skill folder, start the server (`gx server start`) against the throwaway home;
   the new links exist after start, with no manual install.
10. Story 10: after story 4, bare `gx skills install` keeps the `--copy` source as copies and the
    link source as links.
11. Story 11: rerun `gx skills install <src2>` without `--copy` and without `--force`; exit 0 and
    its entries become links. Rerun with `--copy`; they become copies again. Change one copied file
    first: exit non-zero, no entry switches, the record keeps `--copy`, and `--force <path>`
    then completes the switch.
12. Story 12: `gx skills install <symlink-to-src>`; `gx skills list` still shows one source.
13. Story 13: a second source with a skill of the same name as one in `<src>`: install exits
    non-zero, names both sources, skips that name in both (no entry for it in either root, and an
    earlier link for it from `<src>` is removed), and installs the other skills of both. The same
    result with one source in link mode and one in `--copy` mode. `--force-all` changes nothing.
14. Story 14: a source holding an old `gx-verify` copy exits non-zero, names the bundle and the
    source, tells the person to delete the old copy, and skips only the source's `gx-verify`. The
    bundle's `gx-verify` is installed. A source installed before the bundle gets the same result.
15. Story 15: before the first install, create a link in the root that points at the skill's folder;
    install; the link is kept and `gx skills list` counts the skill.
16. Story 16: before the first install, create a dangling link in the root that points into `<src>`;
    install removes it.
17. Story 17: before the first install, create a real folder and a link to somewhere else, each with
    a skill's name; install exits non-zero as a conflict and leaves both. With `--force-all` they
    are replaced.
18. Story 18: create `~/.claude/skills/synced/` before install; it is untouched after install,
    rerun and uninstall.
19. Story 19: make `~/.claude/skills` a symlink to `<src>`; install exits non-zero, prints the
    conversion steps, and `<src>` is byte-for-byte unchanged.
20. Story 20: same check as 19, compared against a checksum of the source folder before and after.
21. Story 21: move `<src>` away; bare install exits non-zero, the links stay, the bundle and the other
    sources still sync, and the output and the web skills problem name `gx skills uninstall <src>`.
22. Story 22: `gx skills uninstall <src>`; its links are gone from both roots, and `gx skills list`
    no longer shows it. Other sources and the bundle are intact.
23. Story 23: replace one installed link with a real folder, run `gx skills uninstall <src>`; that
    entry is kept and reported conflicted; with `--force <path>` it is removed.
24. Story 24: bare `gx skills uninstall` removes the bundle and leaves third-party links.
25. Story 25: `gx skills list` prints the bundle line first, then each source's path, mode, roots
    and skill count; with no sources it says so after the bundle line. Before any bundle install it
    says the bundle is not installed.
26. Story 26: after story 17, `--force <path>` replaces only the named path.
27. Story 27: after story 17 without force, no entry of that source exists in either root. With a
    good second source installed, it is still current. A name clash (story 13) does not abort: the
    source's other skills install.
28. Story 28: with a conflict in a source, `gx server start` prints a skills warning and the web
    UI's skills problem shows it (open the web page and read the banner).
29. Story 29: fix the conflict, run `gx skills install`; the problem file is gone and the web UI
    banner is gone.
30. Story 30: break one source, keep another good; at `gx server start` the good source and the
    bundle are current and the problem names only the broken source. A bare `gx skills install` in
    the same state does the same, prints the problem, writes the joined problem and exits non-zero.
    Then `gx skills install <good-src>` alone leaves the problem record as it was.
31. Story 31: with a same-named folder under `$CODEX_HOME/skills`, install prints the duplicate
    warning for the third-party skill and exits 0.
32. Story 32: `gx skills install --dev` from a checkout behaves as before: the existing dev tests
    pass and third-party links are not disturbed.
33. Story 33: `gx skills install /no/such/dir` and `gx skills install <file>` exit non-zero and
    create no record.
34. Story 34: `gx skills install <empty-dir>` exits 0 and says no skills were found.
35. Story 35: install a source with a skill named like a bundle skill that the test bundle adds
    later (use a test bundle with one extra skill). Bare install installs the bundle's skill, removes
    the source's link for that name, names both owners, exits non-zero, and leaves the source's other
    links. The same with a `--dev` checkout that adds the skill. `--force-all` changes nothing.

Flow-catalog flows: none. The story 28 and 29 banner check uses the web page that shows the skills
problem today; no new flow is needed, since this work adds no screen.

New flows or scenarios to write: none for the web. The CLI checks above are written as command
tests at the seam in Testing Decisions; a verify run repeats them against the temporary server.

## Out of Scope

- Git URL sources.
- A web UI page for managing skills.
- Changing the dotfiles install script.
- Converting `~/.claude/skills` or `~/.codex/skills` for the person. gx prints the steps; the person
  does it.
- Installing into `$CODEX_HOME/skills`. Third-party skills go to `~/.claude/skills` and
  `~/.agents/skills` only.
- Changing how `--dev` works, except that it follows the same name-clash rule as the bundle. gx's own
  checkout is not a third-party source.
- Nested folders. Only a direct subfolder of a source can be a skill.
- Touching entries gx did not record, such as Claude's `synced/` folder.

## Further Notes

- Today the person's `~/.claude/skills` and `~/.codex/skills` are symlinks to
  `~/.dotfiles/core/ai/skills`, and that folder holds old `gx-*` bundle copies and the bundle's
  loose files (`gx.md` and others). The first install of that source refuses on the root overlap,
  and the printed steps include deleting those old copies. If the person converts the root and
  leaves some copies, each one is skipped with an error and the bundle's skill installs.
- The existing pieces this reuses: one manifest per source id, the directory-link entry kind,
  removal of entries a source no longer has, and the destination and source overlap guard.
- Source identity uses the folder's real path, so a symlinked dotfiles folder and its target are one
  source.
