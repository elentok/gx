# iOS Safari AutoFill on free-text textareas

Ticket: gx:ui-fixes/02. Question: which attributes stop iOS Safari from offering credit-card, contact or password AutoFill on a free-text `<textarea>`, keeping the normal keyboard, autocorrect and predictive text.

## Short answer

- `autocomplete="off"` is valid on `<textarea>` and WebKit parses it. But on iOS it only turns into "no text content type", which is the same thing the keyboard gets with no attribute at all. WebKit gives iOS nothing else to say "do not AutoFill".
- The decision to show card suggestions is made by iOS outside WebKit's open source. No primary source documents which signals it uses or whether it honours `off` on a textarea. I found no WebKit bug or Safari release note that fixes or explains this case.
- So the honest recommendation is: set `autocomplete="off"`, add a stable `name`, keep a plain non-financial accessible name, and verify on a real iPhone. A vitest test can only check the attributes.

## What the sources say

### The HTML spec

- The `autocomplete` attribute hints "how to, or indeed whether to, provide such a feature". `off` means the data is sensitive, never reused, or the page has its own autocomplete. The spec says that with `off` the UA "should not remember the control's data, and should not offer past values". The language is advisory. Source: [HTML Standard, autofill](https://html.spec.whatwg.org/multipage/form-control-infrastructure.html#autofill).
- The attribute applies to `input`, `select` and `textarea`. `cc-number`, `cc-csc` and `one-time-code` are all allowed on a textarea, so the spec has no rule that keeps a textarea out of card AutoFill. Same source.
- MDN (secondary) agrees: `<textarea>` supports the attribute. A control with no `autocomplete` inherits its owning form's value. MDN says browsers "may also ignore `autocomplete="off"` on login fields". It does not say anything about card fields. Source: [MDN autocomplete](https://developer.mozilla.org/en-US/docs/Web/HTML/Reference/Attributes/autocomplete).

### WebKit source (read from `main`, Nov 2026)

- `Source/WebCore/html/Autofill.cpp` implements the spec's processing model. `off` is category Off, field name "off". No `autocomplete` attribute gives "on", or "off" when the owning form has `autocomplete=off` (lines near "Label: Default"). Source: [Autofill.cpp](https://github.com/WebKit/WebKit/blob/main/Source/WebCore/html/Autofill.cpp).
- For a focused textarea, `WebPageIOS.mm` sends the UI process `autofillFieldName`, `placeholder`, `title`, `ariaLabel`, the first non-empty `<label>` text, `isAutocorrect`, `autocapitalizeType` and `inputMode`. Source: [WebPageIOS.mm](https://github.com/WebKit/WebKit/blob/main/Source/WebKit/WebProcess/WebPage/ios/WebPageIOS.mm), the `HTMLTextAreaElement` branch of the focused-element information code.
- The login-credential AutoFill path (`AutofillElements::computeAutofillElements`) only accepts `HTMLInputElement`. A textarea is never a login username or password field. Source: [AutofillElements.cpp](https://github.com/WebKit/WebKit/blob/main/Source/WebCore/editing/cocoa/AutofillElements.cpp). So password AutoFill on a textarea is not a WebKit-side path. Contact and card suggestions are.
- In `WKContentViewInteraction.mm`, `contentTypeFromFieldName:` maps field names to `UITextContentType` (name, address, tel, email, url, username, one-time-code). `off`, `on`, `cc-*`, `current-password` and `new-password` all map to `nil`. The result is set as `traits.textContentType`. Source: [WKContentViewInteraction.mm](https://github.com/WebKit/WebKit/blob/main/Source/WebKit/UIProcess/ios/WKContentViewInteraction.mm), `contentTypeFromFieldName:` and `textInputTraits`.
  - Consequence: for `off` and for no attribute, iOS gets the same traits. Any difference must come from iOS itself, which gets the label, aria-label, title and placeholder through the accessibility-style `inputLabelText` and the `_autofillContext` dictionary (page URL, whether login credentials are accepted). I could not read what iOS does with them.
- WebKit's `shouldAutocomplete` (form-level `autocomplete=off`) only controls saving and restoring form state on back navigation. Source: [ValidatedFormListedElement.cpp](https://github.com/WebKit/WebKit/blob/main/Source/WebCore/html/ValidatedFormListedElement.cpp), `isCandidateForSavingAndRestoringState`. It is unrelated to iOS AutoFill.
- Autocorrect and prediction are separate controls: `autocorrect` and `autocapitalize` attributes feed `isAutocorrect` and `autocapitalizeType`. `autocomplete` does not touch them. Setting `autocomplete="off"` therefore keeps the normal keyboard, autocorrect and predictive text. Source: the `HTMLTextAreaElement` branch in WebPageIOS.mm above, and `shouldAutocorrect` in [HTMLFormControlElement.cpp](https://github.com/WebKit/WebKit/blob/main/Source/WebCore/html/HTMLFormControlElement.cpp).

### Bug trackers and release notes

- [WebKit bug 159051](https://bugs.webkit.org/show_bug.cgi?id=159051), "Forms/inputs don't respect autocomplete=off", is RESOLVED INVALID. A 2022 comment calls it "Safari behaviour and not WebKit behaviour". So Safari's own AutoFill layer is where `off` may be ignored, and it is not public.
- [WebKit bug 283719](https://lists.webkit.org/pipermail/webkit-unassigned/2024-November/1197264.html), "AutoFill popup for read-only textarea" (Nov 2024). I could not open the body (403); the title is all I verified. It shows AutoFill UI on a textarea is a known behaviour on iOS 18.
- A 2015 Stack Overflow question reports card AutoFill on textareas that `autocomplete="off"` did not stop (iOS 8, unanswered, weak evidence): [question 30599381](https://stackoverflow.com/questions/30599381).
- [Wikimedia T404912](https://phabricator.wikimedia.org/T404912) (Safari 26, macOS) reports a card popup on a non-card field. The reporter suggests `off` but never confirms it works. Still open.
- The Safari 18.2 WebKit post lists only textarea direction and scrollbar fixes, nothing on AutoFill: [WebKit features in Safari 18.2](https://webkit.org/blog/16574/webkit-features-in-safari-18-2/).
- Apple's user docs say Safari "may be unable to tell which fields" are card fields and offer only a global off switch in Settings. Source: [Apple Support, AutoFill in Safari](https://support.apple.com/guide/iphone/iphccfb450b7/ios).
- I found no WebKit bug or release note that confirms any attribute reliably stops card AutoFill on a textarea.

## The gx code

- `webui/src/ui/expandable-text/ExpandableTextField.tsx`: the Chakra `Textarea` has `aria-label={label}` and a `placeholder`, and no `autocomplete`, `name` or `id`. The chat box passes its own label and placeholder.
- `webui/src/design/NewDesignForm.tsx`: the "Rough idea" `chakra.textarea` has `aria-label="Rough idea"` and a long placeholder, and no `autocomplete`, `name` or `id`. There is no `<form>` element in the file (the ticket assumed one), so there is no form-level value to inherit. The text inputs in the same file (lines 589 and 639) already use `autoComplete="off"`.
- The large editor opened by the expand button is CodeMirror (`CodeEditor.tsx`, a `contenteditable`, not a textarea). `autocomplete` does not apply to it. This research did not check whether iOS shows AutoFill there.

## Recommendation

Both textareas get the same set:

- `autoComplete="off"` (React spelling, renders `autocomplete="off"`). Valid on textarea, parsed by WebKit, matches what the New Design inputs already use. Safe for the keyboard.
- A stable, descriptive `name` that is not financial or contact-like: `name="chat-message"` for the chat box and `name="design-idea"` for the Rough idea box. Reason: with no name and no id, iOS has only the label and placeholder to classify the field. This is a judgement, not a documented rule. The textarea branch of WebPageIOS.mm that I read does not pass `name` or `id` to the UI process, so it may do nothing. It is cheap and harmless.
- Keep `aria-label` and `placeholder` as they are. Neither word set is card-like ("Rough idea", the chat label). Do not add words such as "number", "name", "address", "card", "code" or "phone" to labels or placeholders.
- Leave `autocorrect`, `autocapitalize`, `spellcheck`, `inputmode` and `enterkeyhint` unset so the normal keyboard, autocorrect and predictive text are kept.
- Do not use `autocomplete="one-time-code"`, `new-password`, or an invalid value as a workaround. They either change the keyboard or are ignored. The invalid-value path falls back to "on" in WebKit's code, which is the opposite of what we want.
- Do not add a `<form autocomplete="off">` wrapper. The Rough idea box has no form today, and WebKit's form-level `off` only reaches the textarea as the same "off" field name.

If the real-device check still shows card suggestions with this set, the remaining option is to file it as a Safari bug through Feedback Assistant (bug 159051 says WebKit's tracker will not take it) and accept it.

## What a vitest test can assert

In `ExpandableTextField.test.tsx` and the NewDesignForm test, render the field and assert on the DOM textarea:

- `getAttribute("autocomplete") === "off"`
- `getAttribute("name")` equals the chosen name
- `aria-label` and `placeholder` are unchanged
- none of `autocorrect`, `autocapitalize`, `spellcheck`, `inputmode` are set to off values (so we do not lose the keyboard features)

These pin the markup. They say nothing about Safari.

## What only a real iPhone can check

- Whether the card, contact or password suggestion strip above the keyboard disappears on both textareas, on current iOS Safari and in an installed home-screen web app (they can differ).
- Whether the strip returns after reload or after the user has a saved card.
- Whether autocorrect and the predictive bar still work.
- Whether the CodeMirror large editor shows any AutoFill strip.

Test with a device that has a saved card and contact in Settings > AutoFill, comparing the current build against the recommended attribute set.
