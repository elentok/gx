# Epic page vs mocks: differences

Built page at commit 1d51fed1, driven with `gxfixture` and `playwright-cli` on 2026-10-26. The numbers come from `getComputedStyle` on both pages. Mocks: CombinedRunning, CombinedFinished, CombinedDesign, the three Phone files, LandNotice (option a) and PubStats (option a).

## Scenarios used

| State | Scenario and page | Notes |
| --- | --- | --- |
| Running loop epic | `mixed`, `demo:mixed-epic`, queued with `gx server queue add` | Recovery is on, so 07 became a waiting-for-children parent with a running fork `07a`. This gives a running card and a fork indent, but no needs-repair card. The epic has no Design partner, so there is no "Open design" button. |
| Finished loop epic with Merge | `united`, `demo:awaiting-epic` | State is awaiting-merge. No scenario has a "finished" epic with Merge. |
| Design epic | `design`, `demo:design-epic` | |
| Land-block notice | `united`, `demo:blocked-merge-epic` | Reason "needs a rebase". |
| Draft -impl epic still publishing | `united`, `demo:publishing-epic-impl` | No Design partner, so no "Open design" button. |
| Extra: needs-repair card | `united`, `demo:paused-epic` | No mock exists. The button follows a later decision. |

Fixture edits (fixture state only, never the repo): a `## Summary` section with the mock's text on the loop epics, the mock's Destination on `design-epic`, and `lines_added`, `lines_removed`, `total_tokens` and `created_at` on `mixed-epic`, `awaiting-epic` and `design-epic`, so the More stats card shows. The built Design stats line shows "$0.00", although flow 50 expects no "$". The `total_tokens` stamp may have caused it, so D13 does not count it.

The built page switches to its wide layout at 992px (D1). At 760px it shows the phone layout. So each state also has a `*-1024-built.png` shot of the wide layout, and the desktop numbers below come from that shot.

## Header and ring

**D1. Wide-layout breakpoint.**
- Mock: the 760px desktop mock has the wide layout. The ring is 64px, the summary sits beside the title and the buttons sit right of the stats line.
- Built: `useWide` is `(min-width: 62em)`, which is 992px. At 760px the page has a 52px ring, the summary under the title row and the buttons on their own 44px row.
- Card meta has its own breakpoint. `md` (768px) moves the meta to the title's line (D24).
- Files: `webui/src/design/useWide.ts:7`, `webui/src/epic/EpicPage.tsx:209,234,238,242`.

**D2. Page frame and padding.**
- Mock: no top bar. Padding is 28px 32px 48px on desktop and 20px 16px 40px on phones. The content column is 696px wide.
- Built: the "EPICS STATS" top bar sits above the back link. Padding is 16px on all sides and `maxW` is 1366px, so the column is 992px wide at a 1024px window.
- The mock may leave out the app chrome on purpose. Confirm the content width.
- Files: `webui/src/ui/PageContainer.tsx` (`p="4"`, `maxW="1366px"`), `webui/src/page/DetailPage.tsx:75`.

**D3. Back link.**
- Mock: 14px, colour #a3acba. The gap below it is 28px (24px on phones).
- Built: 16px, white (default link). The gap below it is 16px (`mt="4"`).
- Files: `webui/src/nav/BackLink.tsx:247`, `webui/src/page/PageHeader.tsx:34`.

**D4. Ring and title alignment.**
- Mock: `align-items: center`, gap 18px (14px on phones). The title block is centred on the ring.
- Built: `alignItems="flex-start"`, gap 16px. The title's top lines up with the ring's top.
- File: `webui/src/page/PageHeader.tsx:36`.

**D5. Title size on phones.**
- Mock: phone h1 is 24px, line-height 1.15, `overflow-wrap: anywhere`. Desktop h1 is 30px, line-height 1.1 (33px).
- Built: 30px at every width, line-height 30px (1.0).
- File: the h1 in `webui/src/nav/JumpMenu.tsx`, rendered from `webui/src/epic/EpicPage.tsx:237`.

**D6. Jump-menu caret and ⌘K hint.**
- Mock: a plain h1.
- Built: the title is the jump menu (spec story 5), with a "▾" caret (16px, #a3acba) and, on wide screens, a "⌘K" hint (13px).
- Follows a later decision (the spec keeps the jump menu). Listed so the person can confirm the caret and hint.
- File: `webui/src/epic/EpicPage.tsx:237`.

**D7. Summary text.**
- Mock: 15px, line-height 1.5, colour #c9d0da, `margin-top` 4px under the title. On phones it is a full-width row under the ring row, 12px below it.
- Built: 16px, line-height 24px, white, `mt="2"` (8px).
- File: `webui/src/epic/EpicPage.tsx:221`.

**D8. Kind badge.**
- Mock: background #0a0d11 (the page colour), no border. 26px with a 14px icon on desktop, 22px with a 12px icon on phones.
- Built: background #141920 (`surface.raised`) with a 2px #141920 border, so it reads as a lighter disc. It is 26px with a 14px icon at both sizes.
- File: `webui/src/ui/progress-ring/ProgressRing.tsx:109-139`.

**D9. Comet shape.**
- Mock: one white segment, 10% of the ring, opacity 0.8.
- Built: 10 segments over 16% of the ring, fading from 0.1 to 1.0 opacity.
- File: `webui/src/ui/progress-ring/ProgressRing.tsx:24-27,63-92`.

**D10. Running ring tone.**
- Mock: the running epic's ring is running-blue (#6cb4ff) with the comet.
- Built: `mixed-epic` (running, with a needs-answer ticket) shows a waiting-yellow (#f2b845) arc with the comet, because the joined card's `ringTone` is waiting.
- Follows a later decision (spec decision 7, which takes the tone from the card).
- File: `webui/src/epic/ring.ts:217`.

## Stats bar and buttons

**D11. Hairline and spacing.**
- Mock: `border-top` 1px #252c36, `padding-top` 14px (12px on phones), 14px gap above (12px on phones).
- Built: `border-top` 1px #3d4654 (the `border` token), `pt="3"` (12px), `mt="4"` (16px).
- File: `webui/src/epic/EpicPage.tsx:132`.

**D12. Stats line colour.**
- Mock: 14px, #a3acba, line-height normal.
- Built: 14px, #c9d0da (`text.muted`), line-height 21px.
- The two pages map the names in reverse. The mock's dim grey is the theme's `text.dim` (#a3acba), but the page uses `text.muted`, which is #c9d0da.
- File: `webui/src/epic/EpicPage.tsx:134`.

**D13. State word colour on Design and publishing epics.**
- Mock: "Resolve tickets" and "Publishing tickets" are #c9d0da, one step brighter than the #a3acba line.
- Built: the Design step word has no tone, so it is the same #c9d0da as the rest of the line and does not stand out. "Publishing tickets" uses the muted tone, #b4bcc8.
- File: `webui/src/epic/rows.ts:65,98`, `webui/src/epic/EpicPage.tsx:138`.

**D14. Awaiting-merge state word.**
- Mock: the finished mock with Merge reads "Finished" in green (#55d996), then "took 4h 40m".
- Built: an awaiting-merge epic reads "Awaiting merge" in waiting-yellow (#f2b845).
- File: `webui/src/epic/rows.ts:54-61,67` (`STATE_TONES["awaiting-merge"] = "waiting"`, `epicBadge`).

**D15. Publishing stats line text.**
- Mock: "Publishing tickets · 7 tickets so far".
- Built: "Publishing tickets · $0.00 · 0s agent time". There is no ticket count, and it shows the cost and agent time.
- File: `webui/src/epic/rows.ts:71-80`.

**D16. Button style (neutral).**
- Mock: 38px tall (44px on phones), padding 0 16px, radius 10px, 14px weight 400, transparent background, 1px #3d4654 border, white text.
- Built: 40px tall (44px on phones through the bar's CSS override), radius 8px, 15px weight 600, background #141920.
- Files: `webui/src/ui/button/Button.tsx:163,178-186`, `webui/src/epic/EpicPage.tsx:151`.

**D17. Primary button style (Start design session, Answer).**
- Mock: tinted. Background rgba(108,180,255,0.16), border rgba(108,180,255,0.45), text #6cb4ff, weight 400.
- Built: Start design session is solid #6cb4ff with dark #0a0d11 text, weight 600.
- Files: `webui/src/design/SessionControls.tsx` (`StartDesignButton`), `webui/src/ui/button/Button.tsx:162`.

**D18. Merge button style.**
- Mock: tinted. Background rgba(85,217,150,0.14), border rgba(85,217,150,0.45), text #55d996.
- Built: solid #55d996 with dark text, weight 600, 40px tall, radius 8px.
- Files: `webui/src/epics/MergeButton.tsx`, `webui/src/ui/button/Button.tsx:162`.

**D19. Disabled Queue button.**
- Mock: outline style. Transparent background, border rgba(108,180,255,0.45), text #6cb4ff, opacity 0.45, `cursor: not-allowed`.
- Built: solid #6cb4ff with dark text, opacity 0.5, `cursor: default`.
- Files: `webui/src/epic/EpicPage.tsx:157`, `webui/src/ui/button/Button.tsx:190`.

## Ticket cards

**D20. Card container missing.**
- Mock: each live ticket is a card. Background #141920, 1px #252c36 border, radius 12px, height 48px, padding 0 16px, gap 12px.
- Built: a bare link with no background, no border and a 2px radius (Chakra link default). One line is 23px tall.
- File: `webui/src/epic/TicketCard.tsx:102`.

**D21. Running card border.**
- Mock: the running card's border is rgba(108,180,255,0.40).
- Built: no border.
- File: `webui/src/epic/TicketCard.tsx:78,102`.

**D22. Expanded (needs-answer) card.**
- Mock: tinted card. Background rgba(255,112,102,0.07), border 1px rgba(255,112,102,0.35), radius 12px, padding 12px 16px, gap 10px. The title is weight 500. The meta "needs answer" is right-aligned in #ff7066. The second row is indented 60px, with the 14px question (#c9d0da) filling it and Answer on the right.
- Built: no card chrome. The title is weight 400 in tone colour. The meta is in #c9d0da. The second row is indented 24px (`pl="6"`), with the question followed directly by the button.
- The needs-repair card has no mock. It should probably take the same tinted card in the deadlocked tone.
- File: `webui/src/epic/TicketCard.tsx:79-99`.

**D23. Answer and "Fix it with an agent" buttons.**
- Mock: Answer is a tinted blue primary button, 38px tall (44px and full width on phones), radius 10px, weight 400.
- Built: solid paused-red (#ff7066) with dark text, `size="sm"` (32px), radius 8px, weight 600, at every width. "Fix it with an agent" uses the same style.
- The fix button itself follows a later decision. Its look has no mock.
- File: `webui/src/epic/TicketCard.tsx:89,94`.

**D24. Meta position and style.**
- Mock (desktop): the meta is right-aligned at the card's end, 13px, #a3acba, `white-space: nowrap`. The title ellipsizes (`.ttl`).
- Built: the meta follows the title with an 8px gap. It is 14px, #c9d0da, and it is not right-aligned. Under 768px it wraps to its own line. The title never ellipsizes.
- File: `webui/src/epic/TicketCard.tsx:37-56`.

**D25. Meta colour on running and needs-answer cards.**
- Mock: the meta takes the ticket tone. Running is #6cb4ff ("running · 4m · $0.62") and needs-answer is #ff7066.
- Built: always #c9d0da.
- File: `webui/src/epic/TicketCard.tsx:50`.

**D26. Meta wording.**
- Mock: "running · 4m · $0.62" and "needs answer".
- Built: "claimed · 3m" and "needs-answer · Waiting for your answer". The built page prints the raw rendered status and the pause reason.
- Follows a later decision (spec: other statuses keep their rendered word and pause reason). Listed for confirmation.
- File: `webui/src/epic/rows.ts:161-188`.

**D27. Id style.**
- Mock: monospace (ui-monospace) 12px, #a3acba, fixed width 28px (26px on phones), so titles line up.
- Built: Saira Condensed 14px in the tone colour, with no fixed width. Titles start at different x (59px to 61px).
- File: `webui/src/epic/TicketCard.tsx:39`.

**D28. Title colour.**
- Mock: white (#fff) for live cards. Only the dot carries the tone.
- Built: the tone colour (yellow for waiting, red for asking, blue for running, grey for muted).
- File: `webui/src/epic/TicketCard.tsx:42`.

**D29. Dot and running spinner.**
- Mock: an 8px dot placed directly in the row, 12px gap. Running is an SVG ring of 10px with a 30% arc on a rgba(108,180,255,0.3) track.
- Built: the mark sits in a 16px box, 8px column gap. Running is a 12px CSS border arc (top and right borders only, no track), spinning at 1.05s.
- Files: `webui/src/epic/TicketCard.tsx:13-24,37`, `webui/src/ui/arc/Arc.tsx`.

**D30. Gap between cards.**
- Mock: 8px.
- Built: 12px (`gap="3"`), and 16px (`mt="4"`) above the list. The mock has 28px between the bar and the list.
- File: `webui/src/epic/EpicPage.tsx:71`.

**D31. Fork indent.**
- Mock: 28px on desktop, 20px on phones, with the card narrowed by the same amount.
- Built: 24px at every width (`ml={indent * 6}`).
- File: `webui/src/epic/TicketCard.tsx:78`.

## Done divider and "Show N more"

**D32. Divider.**
- Mock: label 13px #a3acba, lines 1px #252c36, `margin` 16px 0 4px (0 0 4px when it heads the list).
- Built: label 14px #c9d0da, lines #3d4654, 12px gap to the cards, 16px above the group.
- File: `webui/src/epic/EpicPage.tsx:49-54`.

**D33. Done card.**
- Mock: a card with no fill. Border 1px #1d232c, radius 12px, height 44px, padding 0 16px. The title is 14px #c9d0da, the id is monospace #a3acba, and the meta is right-aligned 13px #a3acba.
- Built: a bare link (no border). The id and title are green (#55d996), the title is 15px, and the meta follows the title.
- File: `webui/src/epic/EpicPage.tsx:56`, `webui/src/epic/TicketCard.tsx:102`.

**D34. Done check mark.**
- Mock: a plain 12px check stroke (#55d996, width 2.2) with no circle.
- Built: `StatusIcon kind="done"`, a 16px check inside a circle.
- File: `webui/src/epic/TicketCard.tsx:17`.

**D35. "Show N more" button.**
- Mock: a centred text link. 14px #6cb4ff, no border or background, padding 10px 16px (12px 16px on phones).
- Built: a full-width secondary `Button size="sm"`. It is 32px tall, white 14px weight 600 text, background #141920, border #3d4654, radius 8px.
- File: `webui/src/epic/EpicPage.tsx:59-61`.

## More stats card

**D36. Card chrome.**
- Mock: background #141920, border 1px #252c36, radius 12px, padding 14px 16px, gap 14px. There are 28px above it.
- Built: no background, border 1px #3d4654, radius 6px (`md`), button padding 8px 12px, gap 8px, 16px above it.
- File: `webui/src/epic/MoreStats.tsx:120-133`.

**D37. Chevron.**
- Mock: a 16px SVG chevron, stroke #a3acba.
- Built: the text glyph "▸"/"▾", 16px, white.
- File: `webui/src/epic/MoreStats.tsx:134-136`.

**D38. Label and preview type.**
- Mock: "More stats" is 15px weight 500. The preview is 13px #a3acba, right-aligned.
- Built: "More stats" is 16px weight 400. The preview is 14px #c9d0da (right-aligned through `ml="auto"`).
- File: `webui/src/epic/MoreStats.tsx:137-140`.

**D39. Phone layout of the card.**
- Mock: two rows. The chevron and "More stats" are on top, and the preview is under them with a 28px left pad, 4px gap, padding 12px 14px.
- Built: one row. "More stats" wraps onto two lines ("More / stats") beside a two-line preview.
- File: `webui/src/epic/MoreStats.tsx:121-141`.

**D40. Preview wording.**
- Mock (Design): "3 sessions · 2.1M tokens · step 2 of 8". Mock (loop): "... · lead time 2d".
- Built (Design): "2.1M tokens · lead time 2d so far". Built (loop): "... lead time 2d so far" on an unmerged epic.
- The Design wording follows a later decision (spec story 34). The "so far" suffix comes from the data and is in neither mock.
- File: `webui/src/epic/stats.ts:264-281`.

## Land-block and publishing states

**D41. Land-block notice style.**
- Mock: a tinted box. Background rgba(255,112,102,0.10), radius 10px, padding 10px 14px, gap 10px, a 16px info-circle icon (#ff7066), and text 14px line-height 1.5 in #ffb3ad. It sits 14px under the bar.
- Built: plain text, 14px #ff7066, `mt="2"` (8px), with no box and no icon.
- File: `webui/src/epic/EpicPage.tsx:169-173`.

**D42. Land-block wording.**
- Mock: "Can't merge: <reason>".
- Built: the bare `land_block_reason` ("needs a rebase").
- File: `webui/src/epic/EpicPage.tsx:171`.

**D43. Merge button on a merge-blocked epic.**
- Mock: no Merge button. Only "Open design" shows. The spec (story 14) agrees.
- Built: a solid green Merge button shows.
- This is a recorded implementer deviation (impl tickets 03 and 14, "isMergeable" kept so Merge acts as the retry), not a decision made with the person. Confirm it.
- Files: `webui/src/epic/EpicPage.tsx:164`, `webui/src/epics/MergeButton.tsx:5-6`.

**D44. Publishing Queue button.** This is covered by D19. The fixture has no Design partner, so the "Open design" button order could not be checked live. The code puts it first (`EpicPage.tsx:150`), which matches the mock.

## Phone layout

**D45. Phone header.**
- Mock: the ring (52px) and the title (24px) are centred with a 14px gap. The summary is under them, 12px below. The bar's `padding-top` is 12px.
- Built: the ring is 52px, aligned to the top with a 16px gap. The title is 30px. The summary is 8px below, 16px white.
- Files: `webui/src/page/PageHeader.tsx:36`, `webui/src/epic/EpicPage.tsx:221,242`.

**D46. Phone cards.**
- Mock: two-line cards. Padding 10px 14px, row gap 2px, line gap 10px, meta indented 54px (13px #a3acba), title ellipsized on one line.
- Built: bare links, meta indented 24px (14px #c9d0da). Long titles wrap instead of ellipsizing.
- File: `webui/src/epic/TicketCard.tsx:37-56`.

**D47. Phone needs-answer card.**
- Mock: three rows under the title (meta, question, Answer), each indented 54px. Answer is full width (274px) and 44px tall.
- Built: the meta and question are indented 24px. Answer is 75px by 32px.
- File: `webui/src/epic/TicketCard.tsx:82-97`.

**D48. Phone done cards.**
- Mock: two-line bordered cards (#1d232c), padding 9px 14px, meta indented 54px.
- Built: one-line bare links with no border (the meta, when present, wraps under the title).
- File: `webui/src/epic/TicketCard.tsx:102`.

Phone bar buttons match the mock in height (44px each, on their own row under the stats line). The style differences in D16 to D19 still apply.
