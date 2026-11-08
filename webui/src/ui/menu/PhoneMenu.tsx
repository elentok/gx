import { Box, chakra, Portal, Text } from "@chakra-ui/react";
import {
  cloneElement,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type MouseEvent,
  type ReactElement,
  type ReactNode,
} from "react";
import { useOnCloseSignal } from "../closeSignal";
import type { MenuEntry, MenuProps, SubmenuItem } from "./Menu";

const WIDTH = 280;
const INSET = 8;
const GAP = 4;

interface Row {
  key: string;
  role: "menuitem" | "menuitemcheckbox" | "menuitemradio";
  label: ReactNode;
  hint?: ReactNode;
  inlineHint?: boolean;
  disabled?: boolean;
  disabledReason?: string;
  danger?: boolean;
  checked?: boolean;
  chevron?: boolean;
  current?: boolean;
  run: () => void;
}

type Node = { row: Row } | { separator: string } | { content: ReactNode; key: string };

interface Place {
  left: number;
  maxHeight: number;
  top?: number;
  bottom?: number;
}

/** Below the button when it fits, else above (or the roomier side); the right edge sits on the button's, kept inside the screen. */
function place(rect: DOMRect, height: number): Place {
  const left = Math.min(Math.max(rect.right - WIDTH, INSET), window.innerWidth - WIDTH - INSET);
  const below = window.innerHeight - rect.bottom - GAP - INSET;
  const above = rect.top - GAP - INSET;
  if (height <= below || below >= above) return { left, top: rect.bottom + GAP, maxHeight: below };
  // Anchored by its bottom edge, so a submenu swap grows upward and the panel keeps its place.
  return { left, bottom: window.innerHeight - rect.top + GAP, maxHeight: above };
}

/** The phone menu: an anchored popover over a light dim, with submenus swapped into the same panel. */
export function PhoneMenu({ trigger, items, onClosed, closeSignal }: MenuProps) {
  const [anchor, setAnchor] = useState<{ rect: DOMRect; el: HTMLElement } | null>(null);
  const [path, setPath] = useState<SubmenuItem[]>([]);
  const [active, setActive] = useState<string>();
  const [placed, setPlaced] = useState<Place>();
  const panel = useRef<HTMLDivElement>(null);
  const autoId = useId();
  const tp = trigger.props as { id?: string; onClick?: (e: MouseEvent<HTMLElement>) => void };
  const triggerId = tp.id ?? autoId;

  const reset = () => {
    setAnchor(null);
    setPath([]);
    setActive(undefined);
    setPlaced(undefined);
  };
  const close = () => {
    anchor?.el.focus();
    reset();
    onClosed?.();
  };

  useOnCloseSignal(closeSignal, reset);

  useLayoutEffect(() => {
    if (anchor && !placed && panel.current) setPlaced(place(anchor.rect, panel.current.getBoundingClientRect().height));
  }, [anchor, placed]);
  // The button's row can move while the menu is open (live reorder): follow the button, not the rect captured at open.
  const isPlaced = !!placed;
  useEffect(() => {
    if (!anchor || !isPlaced) return;
    let last = anchor.rect;
    let frame = requestAnimationFrame(function follow() {
      const rect = anchor.el.getBoundingClientRect();
      if (panel.current && (rect.top !== last.top || rect.left !== last.left)) {
        last = rect;
        setPlaced(place(rect, panel.current.getBoundingClientRect().height));
      }
      frame = requestAnimationFrame(follow);
    });
    return () => cancelAnimationFrame(frame);
  }, [anchor, isPlaced]);
  // A browser refuses to focus the panel while it is still hidden, so wait until it is placed.
  useEffect(() => {
    if (anchor && placed) panel.current?.focus();
  }, [anchor, placed]);

  const enter = (s: SubmenuItem) => {
    setPath([...path, s]);
    setActive(undefined);
  };
  const back = () => {
    setPath(path.slice(0, -1));
    setActive(undefined);
  };

  const nodes: Node[] = [];
  const entries: MenuEntry[] = path.length ? path[path.length - 1].items : items;
  if (path.length) {
    nodes.push({ row: { key: "__back", role: "menuitem", label: "‹ Back", run: back } }, { separator: "__back-sep" });
  }
  entries.forEach((e, i) => {
    switch (e.kind) {
      case "separator":
        nodes.push({ separator: `sep-${i}` });
        break;
      case "content":
        nodes.push({ content: e.content, key: e.value });
        break;
      case "checkbox":
        nodes.push({
          row: {
            ...e,
            key: e.value,
            role: "menuitemcheckbox",
            run: () => {
              e.onCheckedChange(!e.checked);
              if (e.closeOnSelect !== false) close();
            },
          },
        });
        break;
      case "radio-group":
        for (const o of e.options) {
          nodes.push({
            row: {
              ...o,
              key: `${e.value}/${o.value}`,
              role: "menuitemradio",
              checked: o.value === e.current,
              run: () => {
                e.onValueChange(o.value);
                close();
              },
            },
          });
        }
        break;
      case "submenu":
        nodes.push({ row: { ...e, key: e.value, role: "menuitem", chevron: true, run: () => enter(e) } });
        break;
      default:
        nodes.push({
          row: {
            ...e,
            key: e.value,
            role: "menuitem",
            run: () => {
              e.onSelect();
              if (e.closeOnSelect !== false) close();
            },
          },
        });
    }
  });
  const rows = nodes.flatMap((n) => ("row" in n && !n.row.disabled ? [n.row] : []));

  const onKeyDown = (e: KeyboardEvent) => {
    const at = rows.findIndex((r) => r.key === active);
    const go = (i: number) => setActive(rows[(i + rows.length) % rows.length]?.key);
    const row = rows[at];
    const handled = (() => {
      switch (e.key) {
        case "ArrowDown":
          return go(at + 1), true;
        case "ArrowUp":
          return go(at < 0 ? -1 : at - 1), true;
        case "Home":
          return go(0), true;
        case "End":
          return go(-1), true;
        case "Enter":
        case " ":
          return row?.run(), true;
        case "ArrowRight":
          return row?.chevron && row.run(), true;
        case "ArrowLeft":
          return path.length && back(), true;
        case "Escape":
          return close(), true;
        default:
          return false;
      }
    })();
    if (handled) {
      e.preventDefault();
      e.stopPropagation();
    }
  };

  return (
    <>
      {cloneElement(trigger as ReactElement<Record<string, unknown>>, {
        id: triggerId,
        "aria-haspopup": "menu",
        "aria-expanded": !!anchor,
        onClick: (e: MouseEvent<HTMLElement>) => {
          tp.onClick?.(e);
          if (e.defaultPrevented) return;
          if (anchor) close();
          else setAnchor({ rect: e.currentTarget.getBoundingClientRect(), el: e.currentTarget });
        },
      })}
      {anchor && (
        <Portal>
          <Box position="fixed" inset="0" zIndex="popover" pointerEvents="auto" data-testid="menu-dim" bg="rgba(0,0,0,.35)" onClick={close}>
            <Box
              ref={panel}
              role="menu"
              aria-labelledby={triggerId}
              tabIndex={-1}
              onKeyDown={onKeyDown}
              onClick={(e) => e.stopPropagation()}
              position="absolute"
              w={`${WIDTH}px`}
              p="1"
              overflowY="auto"
              bg="surface.raised"
              borderWidth="1px"
              borderColor="border"
              borderRadius="12px"
              boxShadow="0 12px 32px rgba(0,0,0,.55)"
              outline="none"
              visibility={placed ? "visible" : "hidden"}
              style={placed ?? { left: 0, top: 0 }}
            >
              {nodes.map((n) => {
                if ("separator" in n) return <Box key={n.separator} role="separator" h="1px" bg="border" mx="1.5" my="1" />;
                if ("content" in n) return <Box key={n.key}>{n.content}</Box>;
                const r = n.row;
                return (
                  <chakra.button
                    type="button"
                    key={r.key}
                    role={r.role}
                    aria-checked={r.role === "menuitem" ? undefined : !!r.checked}
                    aria-current={r.current}
                    aria-disabled={r.disabled || undefined}
                    data-disabled={r.disabled ? "" : undefined}
                    data-highlighted={r.key === active ? "" : undefined}
                    onClick={r.disabled ? undefined : r.run}
                    display="flex"
                    w="full"
                    minH="48px"
                    ps="3"
                    pe="3"
                    gap="10px"
                    textAlign="left"
                    borderRadius="8px"
                    fontWeight="600"
                    fontSize="15px"
                    position="relative"
                    title={r.disabled ? r.disabledReason : undefined}
                    cursor={r.disabled ? "not-allowed" : "pointer"}
                    color={r.disabled ? "text.muted" : r.danger ? "tone.paused.fg" : "text"}
                    {...(r.inlineHint
                      ? { flexDirection: "row", alignItems: "center", justifyContent: "space-between" }
                      : { flexDirection: "column", alignItems: "flex-start", justifyContent: "center" })}
                    _highlighted={{ bg: "surface" }}
                    _active={{ bg: "surface" }}
                  >
                    <Text as="span">{r.label}</Text>
                    {(r.disabled && r.disabledReason ? r.disabledReason : r.hint) && (
                      <Text as="span" fontSize="xs" fontWeight="400" color="text.muted">
                        {r.disabled && r.disabledReason ? r.disabledReason : r.hint}
                      </Text>
                    )}
                    {r.chevron && (
                      <Text as="span" aria-hidden position="absolute" right="4">
                        ›
                      </Text>
                    )}
                    {r.checked && (
                      <Text as="span" aria-hidden position="absolute" right="4">
                        ✓
                      </Text>
                    )}
                  </chakra.button>
                );
              })}
            </Box>
          </Box>
        </Portal>
      )}
    </>
  );
}
