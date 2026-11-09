import { Box, Menu as ChakraMenu, Portal, Text } from "@chakra-ui/react";
import { Fragment, useState, type ReactElement, type ReactNode } from "react";
import { useWide } from "../../useWide";
import { useCloseSignal, useOnCloseSignal } from "../closeSignal";
import { PhoneMenu } from "./PhoneMenu";
import { useReportOpen } from "./useReportOpen";

interface ItemBase {
  /** Unique within the menu; also the key. */
  value: string;
  label: ReactNode;
  /** A smaller line under the label, or at the right edge with `inlineHint`. */
  hint?: ReactNode;
  inlineHint?: boolean;
  disabled?: boolean;
  /** Why the item is disabled; shown as its hint and as its title. */
  disabledReason?: string;
}

export interface ActionItem extends ItemBase {
  kind?: "action";
  danger?: boolean;
  /** Marks the item that is the current choice (aria-current); false marks it as not current. */
  current?: boolean;
  /** False keeps the menu open after the pick. */
  closeOnSelect?: boolean;
  onSelect: () => void;
}

export interface CheckboxItem extends ItemBase {
  kind: "checkbox";
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  closeOnSelect?: boolean;
}

export interface RadioGroupItem {
  kind: "radio-group";
  /** Names the group; also the key. */
  value: string;
  current: string;
  onValueChange: (value: string) => void;
  options: { value: string; label: ReactNode; hint?: ReactNode; disabled?: boolean }[];
}

export interface SubmenuItem extends Omit<ItemBase, "disabled" | "disabledReason"> {
  kind: "submenu";
  items: MenuEntry[];
}

export interface SeparatorItem {
  kind: "separator";
}

/** A free-form row (a key-value block, a row with its own button). */
export interface ContentItem {
  kind: "content";
  value: string;
  content: ReactNode;
}

export type MenuEntry = ActionItem | CheckboxItem | RadioGroupItem | SubmenuItem | SeparatorItem | ContentItem;

/** Joins groups of entries into one list with a rule between non-empty groups. */
export function menuGroups(groups: MenuEntry[][]): MenuEntry[] {
  return groups.filter((g) => g.length > 0).flatMap((g, i) => (i > 0 ? [{ kind: "separator" } as const, ...g] : g));
}

/** The one place item styling and focus state are drawn. */
const itemStyle = {
  cursor: "pointer",
  color: "text",
  ps: "3",
  fontWeight: "600",
  fontSize: "15px",
  _highlighted: { bg: "surface" },
  _disabled: { cursor: "not-allowed", color: "text.muted" },
} as const;

function rowLayout(inlineHint?: boolean) {
  return inlineHint
    ? ({ flexDirection: "row", alignItems: "center", justifyContent: "space-between", gap: "10px" } as const)
    : ({ flexDirection: "column", alignItems: "flex-start" } as const);
}

function Label({ item }: { item: ItemBase }) {
  const hint = item.disabled && item.disabledReason ? item.disabledReason : item.hint;
  return (
    <>
      <Text as="span">{item.label}</Text>
      {hint && (
        <Text as="span" fontSize="xs" fontWeight="400" color="text.muted">
          {hint}
        </Text>
      )}
    </>
  );
}

function Entry({ entry }: { entry: MenuEntry }) {
  switch (entry.kind) {
    case "separator":
      return <ChakraMenu.Separator bg="border" mx="1" my="1.5" />;
    case "content":
      return <Box>{entry.content}</Box>;
    case "checkbox":
      return (
        <ChakraMenu.CheckboxItem
          value={entry.value}
          checked={entry.checked}
          onCheckedChange={entry.onCheckedChange}
          closeOnSelect={entry.closeOnSelect}
          disabled={entry.disabled}
          title={entry.disabled ? entry.disabledReason : undefined}
          {...rowLayout(entry.inlineHint)}
          {...itemStyle}
        >
          <Label item={entry} />
          <ChakraMenu.ItemIndicator />
        </ChakraMenu.CheckboxItem>
      );
    case "radio-group":
      return (
        <ChakraMenu.RadioItemGroup value={entry.current} onValueChange={(e) => entry.onValueChange(e.value)}>
          {entry.options.map((o) => (
            <ChakraMenu.RadioItem
              key={o.value}
              value={o.value}
              disabled={o.disabled}
              {...rowLayout(false)}
              {...itemStyle}
            >
              <Label item={o} />
              <ChakraMenu.ItemIndicator />
            </ChakraMenu.RadioItem>
          ))}
        </ChakraMenu.RadioItemGroup>
      );
    case "submenu":
      return (
        <ChakraMenu.Root positioning={{ placement: "right-start", gutter: 2 }} lazyMount unmountOnExit>
          <ChakraMenu.TriggerItem {...rowLayout(true)} {...itemStyle}>
            {entry.inlineHint ? (
              <Label item={entry} />
            ) : (
              <Box display="flex" flexDirection="column" alignItems="flex-start">
                <Label item={entry} />
              </Box>
            )}
            <Text as="span" aria-hidden>
              ›
            </Text>
          </ChakraMenu.TriggerItem>
          <Panel entries={entry.items} />
        </ChakraMenu.Root>
      );
    default:
      return (
        <ChakraMenu.Item
          value={entry.value}
          closeOnSelect={entry.closeOnSelect}
          disabled={entry.disabled}
          title={entry.disabled ? entry.disabledReason : undefined}
          aria-current={entry.current}
          onClick={entry.disabled ? undefined : entry.onSelect}
          {...rowLayout(entry.inlineHint)}
          {...itemStyle}
          {...(entry.danger && { color: "tone.paused.fg" })}
        >
          <Label item={entry} />
        </ChakraMenu.Item>
      );
  }
}

/** Portalled so a scrolling container (the sidebar) cannot clip the menu or its submenus. */
function Panel({ entries }: { entries: MenuEntry[] }) {
  return (
    <Portal>
      <ChakraMenu.Positioner>
        <ChakraMenu.Content bg="surface.raised" borderWidth="1px" borderColor="border" minW="280px">
          {entries.map((e, i) => (
            <Fragment key={"value" in e ? e.value : `sep-${i}`}>
              <Entry entry={e} />
            </Fragment>
          ))}
        </ChakraMenu.Content>
      </ChakraMenu.Positioner>
    </Portal>
  );
}

/**
 * The shared menu. `trigger` is one element (a button) and names the menu: its accessible name
 * becomes the menu's. Changing `closeSignal` closes the menu, so a parent can close every menu
 * opened from it.
 */
export function GxMenu(props: MenuProps) {
  const inherited = useCloseSignal();
  const all = { ...props, closeSignal: props.closeSignal ?? inherited };
  // On a phone the menu is an anchored popover with in-place submenus; placement is a desktop concern.
  return useWide() ? <WideMenu {...all} /> : <PhoneMenu {...all} />;
}

export interface MenuProps {
  trigger: ReactElement;
  items: MenuEntry[];
  onClosed?: () => void;
  /** True while the menu is open, false once it closes or unmounts. */
  onOpenChange?: (open: boolean) => void;
  closeSignal?: unknown;
  placement?: "bottom-end" | "bottom-start" | "top-end" | "top-start";
}

function WideMenu({ trigger, items, onClosed, onOpenChange, closeSignal, placement = "bottom-end" }: MenuProps) {
  const [open, setOpen] = useState(false);
  useReportOpen(open, onOpenChange);
  useOnCloseSignal(closeSignal, () => setOpen(false));
  return (
    <ChakraMenu.Root
      open={open}
      lazyMount
      unmountOnExit
      loopFocus
      positioning={{ placement }}
      onOpenChange={(e) => {
        setOpen(e.open);
        if (!e.open) onClosed?.();
      }}
    >
      <ChakraMenu.Trigger asChild>{trigger}</ChakraMenu.Trigger>
      <Panel entries={items} />
    </ChakraMenu.Root>
  );
}
