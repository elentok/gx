import { Box } from "@chakra-ui/react";
import { type ReactNode, useEffect } from "react";
import { useActionableCount } from "../agents/useActionableCount";
import { SHELL_STRIPS_HEIGHT_VAR } from "../shell/AppShell";
import { useDevice } from "../useWide";
import { PageContainer } from "../ui/PageContainer";
import { formatTitle, type PageTitle } from "./title";
import { FILL_HEIGHT_VAR, useFillHeight } from "./useFillHeight";

// The only place that writes `document.title`. Writes on mount and whenever the string changes;
// the next page sets its own, so nothing is restored.
function usePageTitle(title: PageTitle) {
  const text = formatTitle(title, useActionableCount());
  useEffect(() => {
    document.title = text;
  }, [text]);
}

// The frame every page shares: the page container under the shell's strips.
// `fill` makes the page exactly the window tall; its children then share that height.
export function DetailPage({
  fill,
  title,
  children,
}: {
  title: PageTitle;
  fill?: boolean;
  children: ReactNode;
}) {
  usePageTitle(title);
  useFillHeight(fill);
  // On phone and tablet the page is fixed to the window. A fixed element touching the top edge keeps iOS 26 from drawing its scroll-edge blur (the fade) over the header.
  const device = useDevice();
  const pinned = fill && device !== "desktop";
  return (
    <Box
      {...(fill && { h: `var(${FILL_HEIGHT_VAR}, 100dvh)`, overflow: "hidden", display: "flex", flexDirection: "column" })}
      {...(pinned && {
        position: "fixed",
        top: `var(${SHELL_STRIPS_HEIGHT_VAR}, 0px)`,
        left: "0",
        right: "0",
        h: `calc(var(${FILL_HEIGHT_VAR}, 100dvh) - var(${SHELL_STRIPS_HEIGHT_VAR}, 0px))`,
      })}
    >
      <PageContainer fill={fill}>{children}</PageContainer>
    </Box>
  );
}
