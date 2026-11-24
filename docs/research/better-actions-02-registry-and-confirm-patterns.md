# Action registry and confirm patterns in React and Chakra

Ticket: gx:better-actions/02. Repo versions: Chakra UI 3.37.0 and Zag.js 1.43.3 (`webui/pnpm-lock.yaml`), React 19 (`webui/package.json`).

Some pages could not be read in full (Apple HIG alerts and Material 3 dialogs returned no body; Primer ActionMenu and ConfirmationDialog guidelines were truncated). Claims below come only from text that was read.

## 1. Registry patterns

### VS Code: declare the command once, place it many times

- A command has a title, optional icon and category, and an `enablement` clause. The Command Palette shows commands by default, and they can appear in other menus. [contribution points](https://code.visualstudio.com/api/references/contribution-points)
- `enablement` applies to all menus and to keybindings. A `when` clause applies to one menu only. [contribution points](https://code.visualstudio.com/api/references/contribution-points)
- A menu entry's `group` sets grouping and sorting. Groups sort lexicographically, `navigation` always sorts first, and `group@<number>` orders items inside a group. [contribution points](https://code.visualstudio.com/api/references/contribution-points)
- The `commandPalette` menu takes a `when` to hide a command from the palette. [contribution points](https://code.visualstudio.com/api/references/contribution-points)
- Keybindings name a command and an optional `when`. A bound key shows up in every UI that shows the command. [contribution points](https://code.visualstudio.com/api/references/contribution-points)
- `when` clauses use `!`, `&&`, `||`, comparisons and `in`, over named context keys that code can set. [when clause contexts](https://code.visualstudio.com/api/references/when-clause-contexts)

Takeaway: one command record owns identity, label and enablement. Each surface adds only placement (group, order) and its own visibility.

### kbar and cmdk: palette-first libraries

- kbar actions are plain data (`id`, `name`, `shortcut`, `keywords`, `perform`), passed as an array to a provider. [kbar README](https://github.com/timc1/kbar/blob/main/README.md) The README does not cover `section`, `priority` or per-page registration, so those were not verified.
- cmdk is declarative JSX: you render `Command.Item` and `Command.Group`, each item with a unique `value`, optional `keywords`, and `onSelect`. It has no central registry. [cmdk README](https://github.com/pacocoursey/cmdk/blob/main/README.md)
- cmdk does not bind a global shortcut for you. The README says to add the key listener yourself for full control over keybind context. [cmdk README](https://github.com/pacocoursey/cmdk/blob/main/README.md)

Takeaway: a palette library renders items. It does not own the model. gx should keep its own registry and map it to palette items.

### Primer and React Aria: menus take data or collections

- React Aria builds a `Menu` from static children or a dynamic `items` array plus a render function. Items have an `id`, a `textValue` for typeahead, an `onAction` handler and `isDisabled`. Sections and separators group items. [React Aria Menu](https://react-aria.adobe.com/Menu)
- Primer's ActionMenu groups with dividers and section titles. A menu with no available items should have an inactive trigger. Inactive items show why through `inactiveText`. [Primer ActionMenu](https://primer.style/product/components/action-menu/), [guidelines](https://primer.style/product/components/action-menu/guidelines/)
- Primer ActionMenu's `preventFocusOnClose` stops focus return to the anchor, and then the consumer must manage focus. [Primer ActionMenu](https://primer.style/product/components/action-menu/)

### Chakra v3 Menu

- `Menu.Item` takes `value`, `onSelect`, `closeOnSelect`, `disabled` and `valueText`. It has no danger or color prop, so danger is styled by the caller. `Menu.ItemGroup` groups, `Menu.ItemCommand` shows a shortcut hint. [Chakra Menu](https://www.chakra-ui.com/docs/components/menu)
- `Menu.Root` accepts `open` and `onOpenChange`. [Chakra Menu](https://www.chakra-ui.com/docs/components/menu)

Gx already wraps this as `GxMenu` with data-driven `MenuEntry` items (ticket 01), so the registry only has to emit `MenuEntry` lists.

## 2. What a registry record should declare

The sources show these fields, spread over several systems:

- Identity and label: VS Code title and category; kbar `name`; React Aria `id` and `textValue`.
- Availability: VS Code `enablement` (global, also for keybindings) plus per-menu `when`; React Aria `isDisabled`; Primer inactive item with a reason.
- Order and grouping: VS Code `group@order`; React Aria sections.
- Shortcut: VS Code keybinding; kbar `shortcut`; Chakra `Menu.ItemCommand` for display.
- Danger: only Primer (`danger` button variant on its dialogs) and the caller's own styling in Chakra. No source defines danger on the command itself, so gx owns that field.
- Bulk fit: none of the sources defines it. VS Code's closest idea is a context key such as the selection count that a `when` clause reads. gx must define bulk fit itself.

## 3. Confirming destructive actions

### Popover or modal

- WAI-ARIA defines an alert dialog as a modal dialog that interrupts the workflow to get a response, for example an action confirmation. It needs `role="alertdialog"`, `aria-modal="true"`, a name (`aria-labelledby` or `aria-label`) and `aria-describedby` for the message. [APG alertdialog](https://www.w3.org/WAI/ARIA/apg/patterns/alertdialog/)
- APG says to mark a dialog modal only if code blocks outside interaction and styling obscures the outside content. [APG alertdialog](https://www.w3.org/WAI/ARIA/apg/patterns/alertdialog/)
- No primary source read says "use a popover for a confirm." The sources show it is allowed. Chakra's `Popover.Content` takes any `role`, and gx's own `design/InterruptPopover.tsx` already uses `role="alertdialog"` inside a Chakra Popover. Chakra `Popover` is non-modal by default, and `modal` adds a focus trap and blocks outside interaction and scroll. [Chakra Popover](https://www.chakra-ui.com/docs/components/popover)

So a confirm popover is a non-modal alert dialog. Where a popover is the confirm, set `modal` on it so it keeps the alert dialog contract (trapped focus, outside inert). A chord or palette has no anchor, so a centered modal dialog fits there.

### Focus rules

- Opening: focus moves into the dialog. For irreversible actions, APG says it may be advisable to focus the least destructive action. [APG modal dialog](https://www.w3.org/WAI/ARIA/apg/patterns/dialog-modal/)
- Primer's ConfirmationDialog focuses the confirm button by default and focuses Cancel when the confirm is dangerous. [Primer ConfirmationDialog](https://primer.style/product/components/confirmation-dialog/)
- Closing: focus returns to the element that invoked the dialog, unless it no longer exists. Then it goes to another element that supports the workflow. [APG modal dialog](https://www.w3.org/WAI/ARIA/apg/patterns/dialog-modal/)
- Escape closes the dialog. Tab wraps inside it. [APG modal dialog](https://www.w3.org/WAI/ARIA/apg/patterns/dialog-modal/)
- Primer asks for specific button labels ("Delete", not "OK"). [Primer ConfirmationDialog](https://primer.style/product/components/confirmation-dialog/)

### Anchoring when the menu item has closed

- A menu item activation closes the menu, so the item is gone. The anchor must be the ⋯ trigger, which stays mounted.
- Chakra Popover supports this: `positioning.getAnchorRect` positions from any element's rect. `initialFocusEl` and `finalFocusEl` set focus in and out, and `restoreFocus` defaults to true. [Chakra Popover](https://www.chakra-ui.com/docs/components/popover)
- Zag documents the same options and says Escape returns focus to the trigger. [Zag popover](https://zagjs.com/components/react/popover)
- React Aria's equivalent is `triggerRef` with controlled `isOpen`, and `getTargetRect` to override the rect. [React Aria Popover](https://react-aria.adobe.com/Popover)
- Chakra says to open a Dialog from a menu item with controlled state, and to use `createOverlay` when a Dialog opened from a menu item contains another menu, to avoid close-on-bubbling bugs. [Chakra Dialog](https://www.chakra-ui.com/docs/components/dialog), [Chakra Menu](https://www.chakra-ui.com/docs/components/menu)
- Chakra Dialog has `initialFocusEl`, `finalFocusEl` and `restoreFocus`. `role` is `dialog` or `alertdialog`. [Chakra Dialog](https://www.chakra-ui.com/docs/components/dialog)
- Gx's `InterruptPopover` is a working local example: controlled `open`, `getAnchorRect` from a ref, `finalFocusEl` set to the anchor, `persistentElements` for the anchor so its own click does not count as outside, and `initialFocusEl` that picks Cancel for a click and the confirm button for a key.

If the menu and the popover both open from the same click, the menu's close would count as an outside interaction. `persistentElements` (used in `InterruptPopover`) handles this for the anchor. A request store, as gx uses for `removeConfirmOpen`, lets the menu close first and the confirm host open next, on the same anchor ref.

## 4. Phones

- Primer Dialog switches placement by breakpoint: `{narrow: 'bottom', regular: 'center'}` gives a bottom sheet on narrow screens. [Primer Dialog](https://primer.style/components/dialog/react/latest)
- Chakra Drawer supports `placement="bottom"`, switched with `mdDown` media queries. [Chakra Drawer](https://www.chakra-ui.com/docs/components/drawer)
- React Aria Popover documents no mobile swap. [React Aria Popover](https://react-aria.adobe.com/Popover)
- So design systems do swap dialogs to sheets, but none of the sources read says a popover must swap on phones. Gx's own `PhoneMenu` already shows an anchored popover works on a phone with edge insets and above/below flip (`ui/menu/PhoneMenu.tsx`). Chakra/Zag positioning flips and shifts by default, but this was not verified in the docs read, so test it in `gx-ui-check` at phone width.

## 5. Recommendations for gx

- One `ActionDef` per action per entity type, as data: `id`, `label`, `danger`, `group`, `order`, `shortcut` (chord key), `available(ctx)`, `confirm(ctx)` and `bulk`. This follows VS Code's split between command and placement. The map's Destination already fixes label, order, availability and confirm.
- Keep availability global on the action (VS Code `enablement`). Add per-surface visibility only as an optional override (VS Code `when`), for example to hide a chord-only action from a card menu. Use the Primer idea of an inactive item with a reason where an action is relevant but blocked.
- Declare `group` and `order` once and have every surface read them, so the sidebar, header, card and bulk bar all show the same sequence. Put destructive actions in their own last group. Chakra has no danger prop, so `GxMenu` keeps owning danger styling from the registry's `danger` field.
- Treat the registry as the model and adapt it to each consumer: `GxMenu` entries, bulk bar buttons, chord handlers and palette rows (cmdk and kbar both stay data-in, items-out). Do not let a palette library own the list. Bind chords yourself, as cmdk's README advises.
- Define bulk fit in the registry because no source does it for us. Each action says which entity kinds it supports in bulk and whether a mixed selection is allowed. The bulk bar shows the intersection.
- Build one `ConfirmHost` that takes the action's `confirm(ctx)` result and an optional anchor ref. With an anchor (menu item or button) it renders a Chakra `Popover` with `modal`, `role="alertdialog"` and `getAnchorRect` on the ⋯ trigger, copying `InterruptPopover`. Without an anchor (chord, palette) it renders a Chakra `Dialog` with `role="alertdialog"` and `initialFocusEl` on Cancel.
- Follow the focus rules in both forms: focus Cancel first for danger, `finalFocusEl` to the ⋯ trigger (the invoking element), and fall back to the page's ⋯ button when the row was removed. Name the dialog and give it `aria-describedby`. Use the action's label as the confirm label ("Remove", not "OK"), and "Cancel" everywhere (today some say "Keep").
- On phones keep the confirm as an anchored popover, per the map. The risk is a popover near a screen edge or the keyboard, so add one phone-width check in `gx-ui-check` and rely on positioning flip. If it fails, fall back to a bottom-placed Dialog for that case only, as Primer does for narrow viewports.
- Open the confirm from a request store, not from inside the menu item handler, so the menu finishes closing first. This avoids the close-on-bubble problem Chakra documents for dialogs opened from menu items and keeps the anchor on the ⋯ trigger.
- Replace `window.confirm`, inline Keep/Delete rows and the per-feature dialogs with the shared confirm. Only keep a confirm where the registry says so (always, never, or a `confirm(ctx)` condition such as live sessions).
