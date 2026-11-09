# ui-fixes/03: can a web page hide the row above the iPhone keyboard

Ticket: `gx:ui-fixes/03`. Researched 2026-11-09. gx code read on branch `ui-fixes-design`; nothing was changed.
Claims marked (unverified) have no primary source, or could not be tested here (no device).

## Short answer

- Neither part of the row can be hidden by a web page. Not the arrows/Done bar, and not the QuickType/AutoFill suggestions. This holds in Safari and in a home-screen web app. I found no page-level control in any source.
- The only hide switch for the accessory bar is a native API for apps that embed WebKit. It is not reachable from page JavaScript or HTML.
- The page can only influence the suggestions indirectly, and on current iOS even that is not reliable.
- A page can always give its own hide-keyboard control with `blur()`. It works when called from a tap handler (unverified on a device).
- Detect the keyboard with `visualViewport` plus focus. gx's hook uses only the `visualViewport` half. Gaps are listed in section 5.

## 1. The arrows/Done bar (form assistant)

- WebKit bug 155574, "[iOS] Allow clients to hide the accessory view on a form input session", is RESOLVED FIXED. The patch added `hidesAccessoryView` on `_WKFormInputSession`, a Cocoa class. The page names only clients (apps), and no web-facing control. <https://bugs.webkit.org/show_bug.cgi?id=155574>
- So the switch exists for a native app that hosts a `WKWebView` (Cordova's `Keyboard.hideFormAccessoryBar` is the known user of this kind of switch). A page in Safari or a home-screen app does not own that view. (Cordova link is from a search result, not read; the bug page above is the primary source.)
- The HTML attributes do not touch it. The spec defines `inputmode` and `enterkeyhint` as choosing the keyboard type and the Enter key label. Support is optional. Neither mentions an accessory bar. <https://html.spec.whatwg.org/multipage/interaction.html#input-modalities:-the-enterkeyhint-attribute>
- `autocorrect`, `autocomplete`, `autocapitalize` and `spellcheck` do not remove it either. Nothing in the spec or in WebKit's bug tracker ties them to the bar.
- Fields outside a `<form>`, `contenteditable`, and standalone mode: I found no source that says any of them changes the bar. (unverified, absence of evidence)
- iOS 26: users report the "Done" text became a checkmark button on the bar. They report no way to remove it. Switching Safari's tab bar from Bottom to Compact hides it until Safari restarts. That is a user setting, not something a page controls. This is a forum thread, so treat it as a weak source. <https://discussions.apple.com/thread/256177528>
- The Safari 18 WebKit feature post does not mention the bar. <https://webkit.org/blog/15865/webkit-features-in-safari-18-0/>. I could not read the Safari 26 release notes (the page returned no body), so a change there is not ruled out. (gap)

## 2. QuickType / AutoFill suggestions

- Until iOS 14, text input traits such as autocorrect off disabled the QuickType bar. WebKit bug 230360 (still NEW) reports that on iOS 15 the bar shows spelling candidates regardless of `autocorrect`, `autocomplete`, `spellcheck` and `autocapitalize` set on a web field. <https://bugs.webkit.org/show_bug.cgi?id=230360>
- Apple's iOS 15 note, quoted in that bug, says native apps hide the bar with `spellCheckingType = No`. That is a UIKit property, not web content. Same source.
- Spec: `autocorrect` is a two-state attribute (`on`/`off`) that controls automatic correction while typing. It says nothing about a suggestions bar. URL, email and password inputs are always Off. <https://html.spec.whatwg.org/multipage/interaction.html#attr-enterkeyhint> (section "Autocorrection")
- WebKit treats `autocorrect` as a standard two-state attribute (bug 289197, INVALID). <https://bugs.webkit.org/show_bug.cgi?id=289197>
- Safari 18 added `writingsuggestions="false"`. That is for Apple Intelligence writing suggestions, not the QuickType bar. <https://webkit.org/blog/15865/webkit-features-in-safari-18-0/>
- AutoFill (contacts, passwords, cards) is driven by `autocomplete` tokens. Setting `autocomplete="off"` reduces AutoFill suggestions but, per bug 230360, does not remove the bar. A field type of password, email or url also changes it. (unverified on current iOS)
- Result: the bar space can stay even when it has nothing to suggest. A page cannot reclaim it.

## 3. What a page loses by trying

- `autocorrect="off"` and `autocomplete="off"` turn off correction and AutoFill for that field. The chat question box needs correction, so keep them on.
- `inputmode="none"` hides the keyboard itself (spec: the user agent picks no virtual keyboard). That is not a way to hide the bar and would make the field untypeable on a phone. <https://html.spec.whatwg.org/multipage/interaction.html#input-modalities:-the-enterkeyhint-attribute> (section "inputmode")
- `enterkeyhint` is safe and useful: it relabels the Enter key (`send`, `done`). It does not change the row.

## 4. A reliable hide-keyboard control

- Nothing in the spec or WebKit docs promises that `element.blur()` dismisses the iOS keyboard. The spec for the focus model says blur removes focus, and WebKit hides the keyboard when the focused editable element loses focus. (unverified on device; no primary source found)
- Recommended control: a visible button in the page (for example a "Hide keyboard" chevron in the footer shown while the keyboard is open). On `pointerdown`/`click` call `(document.activeElement as HTMLElement)?.blur()`.
  - Use `onPointerDown` with `preventDefault()` so the button press does not first move focus and re-open the keyboard (unverified).
  - Call it from a user gesture. Do not call it from a timer after an async step.
- Fallbacks that also work: tapping outside the field (a tap on a non-focusable area), and scrolling in the message list (iOS does not dismiss on scroll for web pages, so add a handler only if wanted). (unverified)
- Test on a real iPhone in both Safari and the home-screen app before relying on it. The gx-ui-check Playwright fixture runs desktop WebKit and has no on-screen keyboard, so it cannot confirm this.

## 5. Detecting that the keyboard is open

What the sources say:

- `visualViewport.height` is the visible height. `offsetTop` is the offset of the visual viewport from the layout viewport. `resize` fires when its scale, width or height changes. <https://drafts.csswg.org/cssom-view/#the-visualviewport-interface>
- MDN: "the on-screen keyboard (OSK) can shrink the visual viewport without affecting the layout viewport." Pinch zoom also shrinks the visual viewport while the layout viewport stays put. <https://developer.mozilla.org/en-US/docs/Web/API/Visual_Viewport_API>
- iOS Safari did not fire the resize event for the keyboard on iOS 12. It was fixed on iOS 13. <https://bugs.webkit.org/show_bug.cgi?id=198347>
- Bug 265578 (NEW): with Safari's UI expanded (address bar at the bottom), the visual viewport changes only at the end of the keyboard animation. <https://bugs.webkit.org/show_bug.cgi?id=265578>
- Bug 230225 (VirtualKeyboard API, NEW): there is no `navigator.virtualKeyboard` in Safari. A commenter says the usual workaround is the `visualViewport` resize event plus input focus. Another says viewport measurements cannot tell when the keyboard is up in some cases, such as an external keyboard or a keyboard that opens without resizing. <https://bugs.webkit.org/show_bug.cgi?id=230225>
- MDN: `VirtualKeyboard` is not Baseline and Safari lacks it. <https://developer.mozilla.org/en-US/docs/Web/API/VirtualKeyboard_API>
- Bug 259770 (RESOLVED FIXED): `interactive-widget` in the viewport meta is partly implemented, reportedly landing in a 27.3 beta. A commenter says `resizes-content` still misbehaves, and that the property was ignored before that beta. Do not depend on it. <https://bugs.webkit.org/show_bug.cgi?id=259770>

Recommended approach, from those sources:

1. Keyboard open = an editable element is focused (`focusin`/`focusout` on text inputs, textareas, `contenteditable`) AND `window.innerHeight - visualViewport.height` is above a threshold.
2. Ignore the gap when `visualViewport.scale > 1` (pinch zoom shrinks the height too).
3. Listen to both `visualViewport` `resize` and `scroll`, and to `focusin`/`focusout`. Expect the resize to land late on some Safari layouts (bug 265578).
4. Treat "no keyboard seen" as a normal state. An external keyboard, or Stage Manager, gives no gap while the field is focused.

## 6. Does gx's hook match?

`useKeyboardOpen` (`webui/src/page/useKeyboardOpen.ts`) does: open = `window.innerHeight - visualViewport.height > 150`, updated on `visualViewport` `resize`. It matches half of the recommended approach (the viewport gap and the 150px threshold). Gaps:

1. No focus check. A pinch zoom or a large visual viewport change with no field focused can read as "keyboard open". `visualViewport.scale` is not checked either, and a zoom above about 1.3x on a phone is enough to cross 150px.
2. No `scroll` listener. `useVisualViewportHeight` (used by `BottomSheet`) does listen to `scroll` for `offsetTop`; the keyboard hook does not. Probably harmless for a pure height check.
3. No `focusout` re-sync. If a late resize is missed (bug 265578), the flag can stay stale. Re-reading on `focusin`/`focusout` with a short delay would heal it.
4. 150px is a guess. It is below the shortest iPhone keyboard but above most browser chrome changes. With Safari's compact/bottom tab bar the collapse of the toolbar can change `innerHeight` by 50 to 100px, still under 150. (unverified, no device)
5. `DesignPage.tsx:144` only uses the boolean to drop chrome. That is the intended use. Nothing there depends on the accessory bar being hidden.
6. `useVisualViewportHeight` (`webui/src/ui/modal/useVisualViewportHeight.ts`) follows `visualViewport.height` and `offsetTop` directly. It matches the sources (MDN: use the visual viewport to position things relative to what is on screen) and needs no change.

## 7. Answer for the design page

- Plan for the bar staying. It cannot be removed from a page. Design the question box to fit above it (about 44px bar plus the suggestion row on iOS).
- Reclaim space elsewhere: when `useKeyboardOpen` is true, hide the header, step strip and any footer extras, and put the question box in the visual viewport (`height = visualViewport.height`).
- Offer a "hide keyboard" button that calls `blur()` so the person can reach the rest of the page. It replaces "Done" in the sense that matters.
- If Done goes away (iOS 26 style checkmark, or a future change), the same button still works. It does not depend on the bar.
- Harden the hook: add the focus check, skip `scale > 1`, and re-sync on `focusout`.
- Verify on a real iPhone (Safari and home-screen). No tool in this repo can show the iOS keyboard.
