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
  children,
}: {
  title: string;
  /** The large icon left of the title. */
  icon?: ReactNode;
  /** One truncated line of text under the title. Nothing renders when it is empty. */
  subtitle?: ReactNode;
  /** Page controls at the right edge: the progress ring or status icon first, then the "..." menu. */
  actions?: ReactNode;
  /** The meta bar under a hairline. Nothing renders when it is empty. */
  meta?: ReactNode;
  children?: ReactNode;
}) {
  const wide = useWide();
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
        <Box display="flex" flexWrap="wrap" alignItems="center" gap={wide ? "14px" : "10px"}>
          <MascotToggle />
          {icon ? (
            <Box data-testid="page-header-icon" flex="none" display="flex" alignItems="center" color="text.dim">
              {icon}
            </Box>
          ) : null}
          {/* A wide action (the Design session button) wraps below instead of squeezing the title. */}
          <Box minW="0" flex="1 1 140px">
            {h1}
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
        <Box data-testid="page-header-meta" mt="10px" pt="8px" borderTopWidth="1px" borderColor="hairline">
          {meta}
        </Box>
      ) : null}
      {children}
    </Box>
  );
}
