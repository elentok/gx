import { Box } from "@chakra-ui/react"
import type { ReactNode } from "react"

/** The form's named fields by name. A field the form does not hold is undefined. */
export const formValues = (form: HTMLFormElement) =>
  Object.fromEntries(new FormData(form)) as Record<string, string>

export function ErrorLine({ children }: { children: ReactNode }) {
  return (
    <Box
      role="alert"
      data-mock-part="error"
      bg="tone.paused.bg"
      color="tone.paused.text"
      borderRadius="md"
      px="3"
      py="2"
      fontSize="sm"
    >
      {children}
    </Box>
  )
}
