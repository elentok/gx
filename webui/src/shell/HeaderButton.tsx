import { chakra } from "@chakra-ui/react"
import type { ComponentProps } from "react"

/** An icon button in the sidebar header. Greyed out with `aria-disabled`, so it still gets pointer and focus events. */
export function HeaderButton(props: ComponentProps<typeof chakra.button>) {
  return (
    <chakra.button
      type="button"
      boxSize="34px"
      flex="none"
      borderRadius="8px"
      display="grid"
      placeItems="center"
      color="text.dim"
      cursor="pointer"
      // Chakra's `_hover` also matches an `aria-disabled` element.
      _hover={props["aria-disabled"] ? undefined : { bg: "surface.raised" }}
      _disabled={{ opacity: 0.5, cursor: "default" }}
      {...props}
    />
  )
}
