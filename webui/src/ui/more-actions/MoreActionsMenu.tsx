import { Box, chakra, Menu, Text } from "@chakra-ui/react";
import { Fragment, type ReactNode } from "react";
import { MenuFrame, menuItemStyle } from "../../design/SessionControls";
import { useWide } from "../../useWide";
import { GrabHandle, sheetPanel } from "../modal/BottomSheet";

export interface MoreActionsItem {
  value: string;
  label: ReactNode;
  /** A smaller line under the label. */
  hint?: ReactNode;
  danger?: boolean;
  onSelect: () => void;
}

/** A section is a group of items; the menu draws a rule between sections. */
export type MoreActionsSection = MoreActionsItem[];

const dots = (
  <svg viewBox="0 0 20 20" aria-hidden width="20" height="20" fill="currentColor">
    <circle cx="4.5" cy="10" r="1.7" />
    <circle cx="10" cy="10" r="1.7" />
    <circle cx="15.5" cy="10" r="1.7" />
  </svg>
);

const hovered = { bg: "surface.raised", color: "text" } as const;
const focused = { outline: "2px solid", outlineColor: "tone.running.fg", outlineOffset: "2px" } as const;
const opened = { ...hovered, boxShadow: "inset 0 0 0 1px var(--chakra-colors-border)" } as const;

/**
 * The header's round "More actions" button and its menu: a popover on a wide screen, a bottom sheet
 * on a phone. The content comes in as sections so a page can add its own items.
 * `pinned` holds the button in a state, so the gallery can show hover, focus and open side by side.
 */
export function MoreActionsMenu({ sections, pinned }: { sections: MoreActionsSection[]; pinned?: "hover" | "focus" | "open" }) {
  const sheetMode = !useWide();
  const item = menuItemStyle(sheetMode);
  return (
    <Menu.Root lazyMount unmountOnExit positioning={{ placement: "bottom-end" }}>
      <Menu.Trigger asChild>
        {/* A phone's hit area is 44px around the 40px circle; the padding is clipped by neither. */}
        <chakra.button
          type="button"
          aria-label="More actions"
          cursor="pointer"
          display="grid"
          placeItems="center"
          boxSizing="border-box"
          w={sheetMode ? "40px" : "36px"}
          h={sheetMode ? "40px" : "36px"}
          borderRadius="full"
          color="text.dim"
          position="relative"
          transition="background .12s, color .12s"
          _before={sheetMode ? { content: '""', position: "absolute", inset: "-2px" } : undefined}
          {...(pinned === "hover" && hovered)}
          {...(pinned === "focus" && focused)}
          {...(pinned === "open" && opened)}
          _hover={hovered}
          _focusVisible={focused}
          _open={opened}
        >
          {dots}
        </chakra.button>
      </Menu.Trigger>
      <MenuFrame sheet={sheetMode}>
        <Menu.Content
          bg="surface.raised"
          borderWidth="1px"
          borderColor="border"
          minW={sheetMode ? "64" : "280px"}
          {...(sheetMode && { ...sheetPanel, pb: "6" })}
        >
          {sheetMode && <GrabHandle my="2" />}
          {sections.map((items, i) => (
            <Fragment key={i}>
              {i > 0 && <Box role="separator" h="1px" bg="border" my="1" />}
              {items.map((it) => (
                <Menu.Item
                  key={it.value}
                  value={it.value}
                  onClick={it.onSelect}
                  flexDirection="column"
                  alignItems="flex-start"
                  {...item}
                  {...(it.danger && { color: "tone.paused.fg" })}
                >
                  <Text as="span">{it.label}</Text>
                  {it.hint && (
                    <Text as="span" fontSize="xs" fontWeight="400" color="text.muted">
                      {it.hint}
                    </Text>
                  )}
                </Menu.Item>
              ))}
            </Fragment>
          ))}
        </Menu.Content>
      </MenuFrame>
    </Menu.Root>
  );
}
