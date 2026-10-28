import { Box, type BoxProps } from "@chakra-ui/react";
import type { ReactNode, Ref } from "react";
import { useWide } from "../../useWide";
import { Modal } from "./Modal";
import { useVisualViewportHeight } from "./useVisualViewportHeight";

/** The sheet panel's corners and width, shared by a Modal and a Menu that sit at the bottom of the screen. */
export const sheetPanel = { w: "full", borderTopRadius: "xl", borderBottomRadius: "0" } as const;

/** The pill at the top of a bottom sheet. */
export const GrabHandle = (props: BoxProps) => (
  <Box aria-hidden data-testid="grab-handle" alignSelf="center" w="36px" h="4px" borderRadius="full" bg="border" flexShrink="0" {...props} />
);

interface BottomSheetProps extends Omit<BoxProps, "children"> {
  label: string;
  onClose: () => void;
  /** Keep the TopBar uncovered. */
  belowTopBar?: boolean;
  /** Follow the visual viewport's height so the on-screen keyboard does not hide the footer; otherwise the sheet is as tall as its content, up to 80%. */
  fillViewport?: boolean;
  /** The panel's props on a wide screen, where this is a centered Modal and not a sheet. */
  desktop?: BoxProps;
  ref?: Ref<HTMLDivElement>;
  children: ReactNode;
}

/** A Modal that is a bottom sheet on a phone, with a grab handle, and a centered dialog on a wide screen. */
export function BottomSheet({ belowTopBar, fillViewport, desktop, children, ...modal }: BottomSheetProps) {
  const sheetMode = !useWide();
  const viewport = useVisualViewportHeight(belowTopBar);
  if (!sheetMode) return <Modal {...modal} {...desktop}>{children}</Modal>;
  return (
    <Modal
      {...modal}
      backdrop={{ inset: undefined, ...viewport.overlay, alignItems: "flex-end", p: "0" }}
      {...sheetPanel}
      {...(fillViewport ? { h: viewport.sheetHeight } : { maxH: "80%", overflow: "auto" })}
    >
      <GrabHandle mt="2" />
      {children}
    </Modal>
  );
}
