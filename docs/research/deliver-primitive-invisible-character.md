# Which invisible character makes Claude warn, typed keys or paste?

Ticket: `gx:deliver-primitive/01`. Claude Code version probed: 2.1.330
(`/Users/david/.local/share/claude/versions/2.1.330`). gx's strip set was checked against 2.1.328
(`server/sessions/prompt/invisible/invisible.go:11`).

## Answer in short

- The exact character in the failing send: **not found**. The sent text on disk is pure ASCII (see
  "The failing send").
- Claude strips on **submit (Enter)**, not on paste. Typed keys and pastes behave the same.
- Claude swallows the Enter ("review and press Enter to send") only when the text **starts with `/`
  or `!`** (or has a bidi character, or is empty after stripping). Plain text is cleaned and sent.
  That is why only `/` skill commands hit the bug.
- Claude's hidden-character set is **wider** than gx's strip set. Characters outside gx's set swallow
  the Enter too (probe: U+FE0F, U+034F, U+3164, U+0007).
- Design: strip alone cannot be the guard. The settle Enter (retry on "our text, input not empty")
  stays needed for a slash send.

## What Claude does (source: the claude 2.1.330 binary)

Read with `rg -a -o` on the binary. Names are minified, so cite them as written.

- Submit path of the main prompt: `let Yg=U_t(Fp,...); if(Yg.removed.removedTotal>0){ ... set the
  input to the cleaned text ... Nl({key:"prompt-invisible-removed", text:mAt(n, bce?"empty":Fj?"review":"cleaned")}) ... if(Fj){return} ... }`.
  So it runs when Enter is pressed, on whatever text is in the box. How the text got there (typed or
  pasted) does not matter.
- `lar(e,o,r)` decides whether to return (swallow the Enter):
  `r!=="prompt" || g==="" || o.removedByClass.bidi>0 || /^[/!]/.test(g) || iU.includes(g)`. A text
  starting with `/` or `!` always swallows. Plain text falls to the `"cleaned"` hint
  ("Removed 1 invisible character from your prompt") and is sent in the same Enter.
- `mAt` builds the hint: `Removed 1 invisible character · review and press Enter to send`.
- The strip is behind a flag: `Z7e()` is `b("tengu_tranquil_cloud", !0)` (default on).
- Set of removed characters, `qdt(cp)` plus `UC`:
  - C0 controls except TAB and LF (U+0000-U+001F minus U+0009, U+000A), DEL and C1 (U+007F-U+009F).
  - U+2028, U+2029.
  - Table `jLs`: U+00AD, U+034F, U+061C, U+115F-U+1160, U+17B4-U+17B5, U+180B-U+180F,
    U+200B-U+200F, U+202A-U+202E, U+2060-U+206F, U+3164, U+FE00-U+FE0F, U+FEFF, U+FFA0,
    U+FFF0-U+FFFB, U+1D173-U+1D17A, U+E0000-U+E0FFF and a few more.
  - Lone surrogates (counted by `Gv`).
  - ZWJ/ZWNJ and variation selectors are kept next to emoji or letters (`keptConditional`).
- Classes (`Kv`): tags, bidi (U+061C, U+200E, U+200F, U+202A-U+202E, U+2066-U+2069), zeroWidth
  (U+200B-U+200D, U+2060, U+FEFF), selectors (U+FE00-U+FE0F, U+E0100-U+E01EF, U+180B-U+180D,
  U+180F), other.

## Probe (claude 2.1.330, `--model haiku`, tmux, scratch dir)

Drove a real TUI in tmux (`tmux -L probe2`). Text: `/cost zz` plus one candidate at the end, then
one Enter. "Paste" is a bracketed paste (`tmux paste-buffer -p`). "Typed" is one key per 120 ms
(`send-keys -l`). Scripts: `scratchpad/probe4.py` (not committed).

| Candidate | Input | After first Enter |
|---|---|---|
| U+200B ZWSP | paste | hint "Removed 1 invisible character · review and press Enter to send", Enter swallowed |
| U+200B ZWSP | typed | same hint, Enter swallowed |
| U+00AD soft hyphen | paste | same hint |
| U+200D ZWJ (trailing) | paste | same hint |
| U+200C ZWNJ (trailing) | paste | same hint |
| U+200E LRM | paste | same hint |
| U+FE0F VS16 | paste | same hint |
| U+034F CGJ | paste and typed | same hint |
| U+3164 Hangul filler | paste | same hint |
| U+0007 BEL | paste | same hint |
| U+2028, U+2029 | paste and typed | no hint. Claude shows the character as a line break in the box (`abc` / `def`) |
| U+00A0 NBSP | paste and typed | no hint |
| U+0085 NEL | paste | no hint |

Plain text (no slash) with U+200B, typed: sent on the first Enter, no swallow. Plain text with
U+2028 pasted: sent on the first Enter.

With the slash menu still open (`/cost` alone), Enter picks the menu entry and hides the hint, so
the first probe rounds were misleading. The `/cost zz` form closes the menu.

## Strip set versus Claude's set

gx strips Unicode category Cf, U+2028, U+2029 and edge ZWJ/ZWNJ
(`server/sessions/prompt/invisible/invisible.go:17-31`). Checked with Go's `unicode` package:

- Inside gx's set and warned by Claude: U+200B, U+00AD, U+FEFF, U+200E, U+200C/U+200D at the edge.
- **Outside** gx's set but warned by Claude: U+FE0F and U+034F (category Mn), U+3164, U+115F, U+FFA0
  (Lo), U+0007 and other C0/C1 controls (Cc), U+17B4, U+180B (Mn), U+E0100 (Mn).
- Inside gx's set but never warned in this probe: U+2028, U+2029 (Claude turns them into a line
  break first). Harmless to strip.
- gx keeps a ZWJ/ZWNJ in the middle of a word. Claude also keeps one next to a letter or an
  emoji. The probe cannot show a zero-width character on screen, so this was not tested.

## The failing send

- The sent text for the chat is in `~/.local/state/gx/draft-history/sent/list.json`. The send at
  13:09Z (`/consult it seems like this issue keeps coming back, ...`, 171 bytes, 171 characters) and
  the send at 13:41Z (`/gx-promote-to-design`) hold only ASCII. The initial report draft
  (`chat-drafts/list.json`, 13:07Z) is ASCII too.
- The source transcript
  (`~/.claude/projects/-Users-david-dev-gx--bare/11abd08a-6dce-4552-b5cc-b23348302188.jsonl`) holds
  the person's report at line 6 ("removed 1 hidden charachter ... the prompt started with a slash
  skill"). It never names the character. The 13 hits of "Removed 1 invisible" in it are the agent's
  own searches.
- `~/.local/state/gx/server.log*` and `gx.log` hold no "stripped invisible" or "invisible" line, so
  gx did not log a strip for that send (the chat path does not call `StripInvisible`; the only caller
  is `server/sessions/prompt/firstprompt.go:517`).
- Not recorded anywhere: the text gx actually typed into the TUI. The stored draft is what the
  person sent, so a character added later (by gx or by Claude's skill expansion) would not show.
  No log shows the typed bytes. The character stays unknown.

## What it means for the design

- Typed versus paste does not matter. Claude checks at Enter. Even a typed slash command warns.
  Slash text over 512 bytes going as a paste changes nothing here.
- A slash command with any hidden character loses its first Enter. Plain text with the same
  character does not.
- gx's `StripInvisible` is too narrow to be the only guard. Widening it to Claude's set would close
  most of the gap: add Mn/Lo members of `jLs` (U+034F, U+115F-U+1160, U+17B4-U+17B5,
  U+180B-U+180F, U+3164, U+FE00-U+FE0F, U+FFA0, U+E0100-U+E01EF), and C0/C1 controls except TAB and
  LF. It would still drift when Claude changes the set (gx's comment pins its set to 2.1.328; this
  probe used 2.1.330 and did not compare the two versions).
- The settle Enter stays: on a trusted "our text, input not empty" read, press Enter again, up to 3
  times. It is the only guard that survives a new character.
- The swallowed Enter only rewrites the box to the cleaned text and shows the hint (source: the
  `return` in the submit path above). In the probe the second Enter emptied the box. The probe did
  not check the command output, so "ran once" rests on the source reading.
