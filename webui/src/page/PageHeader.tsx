import { Box, Heading } from "@chakra-ui/react";
import { type ReactNode, useEffect } from "react";
import { MascotToggle } from "../shell/MascotToggle";
import { useShell } from "../shell/ShellContext";
import { useWide } from "../useWide";

/** The one header every page uses: icon, title, subtitle, actions, then an optional meta bar. */
export function PageHeader({
  title,
  icon,
  subtitle,
  actions,
  meta,
  metaFlush,
  caret,
  children,
}: {
  title: string;
  /** A small decorative "▾" after the title (the epic and design pages). */
  caret?: boolean;
  /** The large icon left of the title. */
  icon?: ReactNode;
  /** One truncated line of text under the title. Nothing renders when it is empty. */
  subtitle?: ReactNode;
  /** Page controls at the right edge: the progress ring or status icon first, then the "..." menu. */
  actions?: ReactNode;
  /** The meta bar under a hairline. Nothing renders when it is empty. */
  meta?: ReactNode;
  /** Drops the room under the hairline, for a meta bar that has nothing to show. */
  metaFlush?: boolean;
  children?: ReactNode;
}) {
  const wide = useWide();
  // Inside the shell the mascot toggle leads the header; a page icon beside it reads as a second toggle.
  const shell = useShell();
  const claimHeader = shell?.claimHeader;
  useEffect(() => claimHeader?.(), [claimHeader]);
  const h1 = (
    <Heading as="h1" fontFamily="inherit" fontWeight="600" fontSize="19px" lineHeight="1.2" overflowWrap="anywhere">
      {title}
    </Heading>
  );
  const rich = icon || subtitle || actions || shell;
  return (
    <Box>
      {rich ? (
        <Box display="flex" flexWrap="wrap" alignItems={subtitle ? "flex-start" : "center"} gap={wide ? "14px" : "10px"}>
          <MascotToggle />
          {icon && !shell ? (
            <Box data-testid="page-header-icon" flex="none" display="flex" alignItems="center" color="text.dim">
              {icon}
            </Box>
          ) : null}
          {/* A wide action (the Design session button) wraps below instead of squeezing the title. */}
          <Box minW="0" flex="1 1 140px">
            {caret ? (
              <Box display="flex" alignItems="baseline" gap="6px">
                {h1}
                <Box as="span" aria-hidden="true" data-testid="page-header-caret" color="text.dim" fontSize="12px">
                  ▾
                </Box>
              </Box>
            ) : (
              h1
            )}
            {subtitle ? (
              <Box
                data-testid="page-header-subtitle"
                mt="2px"
                color="text.dim"
                fontSize={wide ? "15px" : "14px"}
                whiteSpace="nowrap"
                overflow="hidden"
                textOverflow="ellipsis"
              >
                {subtitle}
              </Box>
            ) : null}
          </Box>
          {actions ? (
            <Box data-testid="page-header-actions" flex="none" ml="auto" display="flex" alignItems="center" gap="10px">
              {actions}
            </Box>
          ) : null}
        </Box>
      ) : (
        h1
      )}
      {meta ? (
        <Box data-testid="page-header-meta" mt="10px" pt={metaFlush ? "0" : "8px"} borderTopWidth="1px" borderColor="hairline">
          {meta}
        </Box>
      ) : null}
      {children}
    </Box>
  );
}
