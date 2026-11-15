import { chakra } from "@chakra-ui/react"
import type { ComponentProps } from "react"

/** A button that looks like text: no box, the parent's font, an underline on the dim variant. */
export function LinkButton(props: ComponentProps<typeof chakra.button>) {
  return (
    <chakra.button
      type="button"
      borderWidth="0"
      bg="transparent"
      p="0"
      cursor="pointer"
      {...props}
    />
  )
}
