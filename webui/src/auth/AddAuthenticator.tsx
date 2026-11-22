import { Box, chakra, Text } from "@chakra-ui/react"
import qrcode from "qrcode-generator"
import { useLayoutEffect, useRef } from "react"
import { ErrorLine, Field } from "./AuthFields"

function qrSvg(uri: string): string {
  const qr = qrcode(0, "M")
  qr.addData(uri)
  qr.make()
  return qr.createSvgTag({ scalable: true, margin: 0 })
}

// The SVG comes from our own QR generator, not from user input. This insert is not a sanitiser:
// parsing with DOMParser does not make untrusted markup safe. A layout effect attaches it before
// paint, so the box is never empty on first paint.
function QrBox({ uri }: { uri: string }) {
  const ref = useRef<HTMLDivElement>(null)
  useLayoutEffect(() => {
    const doc = new DOMParser().parseFromString(qrSvg(uri), "image/svg+xml")
    ref.current?.replaceChildren(document.importNode(doc.documentElement, true))
  }, [uri])
  return (
    <Box
      ref={ref}
      bg="white"
      p="3"
      borderRadius="md"
      w="190px"
      h="190px"
      css={{ "& svg": { width: "100%", height: "100%", display: "block" } }}
    />
  )
}

// Groups of four, the way authenticator apps show a key to type by hand.
function spaced(secret: string): string {
  return secret.replace(/(.{4})(?=.)/g, "$1 ")
}

export interface AddAuthenticatorProps {
  secret: string
  /** The otpauth:// URI that the QR code and the link both hold. */
  uri: string
  error?: string
  codeLabel?: string
}

/** QR code, add link, key and code field. The caller supplies the form, heading and submit button. */
export function AddAuthenticator({
  secret,
  uri,
  error,
  codeLabel = "Enter the code it shows",
}: AddAuthenticatorProps) {
  return (
    <>
      <Box data-mock-part="totp-qr" display="flex" justifyContent="center">
        <QrBox uri={uri} />
      </Box>
      <Text data-mock-part="totp-link" fontSize="sm" color="text.dim" textAlign="center">
        <chakra.a href={uri} textDecoration="underline">
          Add to authenticator app
        </chakra.a>
      </Text>
      <Text data-mock-part="totp-key" fontSize="sm" color="text.dim">
        Can't scan? Enter this key: <chakra.code fontFamily="inherit">{spaced(secret)}</chakra.code>
      </Text>
      {error && <ErrorLine>{error}</ErrorLine>}
      <Field
        part="code"
        label={codeLabel}
        name="totp_code"
        error={!!error}
        placeholder="123456"
        inputMode="numeric"
        autoComplete="one-time-code"
        autoFocus
      />
    </>
  )
}
