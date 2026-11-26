import { Box } from "@chakra-ui/react"
import type { InputHTMLAttributes, ReactNode } from "react"
import { TextBox } from "../ui/text-box/TextBox"

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

type FieldProps = Pick<InputHTMLAttributes<HTMLInputElement>, "autoFocus" | "required"> & {
  label: string
}

/** The one-time code from an authenticator app, posted as `totp_code`. */
export function CodeBox({ invalid, ...field }: FieldProps & { invalid?: boolean }) {
  return (
    <Box data-mock-part="code">
      <TextBox
        {...field}
        kind="credential"
        autoComplete="one-time-code"
        inputMode="numeric"
        size="lg"
        name="totp_code"
        invalid={invalid}
        placeholder="123456"
      />
    </Box>
  )
}

export function PasswordBox({
  part,
  ...field
}: FieldProps &
  Pick<InputHTMLAttributes<HTMLInputElement>, "defaultValue" | "minLength"> & {
    /** The `data-mock-part` of the box. */
    part: string
    name: string
    autoComplete: "current-password" | "new-password"
    hint?: string
  }) {
  return (
    <Box data-mock-part={part}>
      <TextBox {...field} kind="credential" size="lg" />
    </Box>
  )
}
