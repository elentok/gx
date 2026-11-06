import { Box, Menu } from "@chakra-ui/react";
import type { ReactNode } from "react";

/** The popover's positioner, or on a phone a dimmed backdrop that holds the menu as a bottom sheet and closes it on a press. */
export function MenuFrame({ sheet, children }: { sheet: boolean; children: ReactNode }) {
  if (!sheet) return <Menu.Positioner>{children}</Menu.Positioner>;
  return (
    <Menu.Context>
      {(menu) =>
        menu.open && (
          <Box
            position="fixed"
            inset="0"
            zIndex="modal"
            bg="overlay"
            display="flex"
            alignItems="flex-end"
            onClick={(e) => e.target === e.currentTarget && menu.setOpen(false)}
          >
            {children}
          </Box>
        )
      }
    </Menu.Context>
  );
}

export const menuItemStyle = (sheetMode: boolean) =>
  ({
    cursor: "pointer",
    color: "text",
    ps: "3",
    _highlighted: { bg: "surface" },
    fontWeight: "600",
    fontSize: "15px",
    ...(sheetMode && { py: "3", minH: "44px" }),
  }) as const;
export type MenuItemStyle = ReturnType<typeof menuItemStyle>;
