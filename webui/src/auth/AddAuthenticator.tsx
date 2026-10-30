import { Box, chakra, Text } from "@chakra-ui/react";
import qrcode from "qrcode-generator";
import { ErrorLine, Field } from "./AuthFields";

export const TOTP_MISMATCH = "That code doesn't match. Check the clock on your phone and try again.";

function qrSvg(uri: string): string {
  const qr = qrcode(0, "M");
  qr.addData(uri);
  qr.make();
  return qr.createSvgTag({ scalable: true, margin: 0 });
}

// Groups of four, the way authenticator apps show a key to type by hand.
function spaced(secret: string): string {
  return secret.replace(/(.{4})(?=.)/g, "$1 ");
}

export interface AddAuthenticatorProps {
  secret: string;
  /** The otpauth:// URI that the QR code and the link both hold. */
  uri: string;
  error?: string;
  codeLabel?: string;
}

/** QR code, add link, key and code field. The caller supplies the form, heading and submit button. */
export function AddAuthenticator({ secret, uri, error, codeLabel = "Enter the code it shows" }: AddAuthenticatorProps) {
  return (
    <>
      <Box data-mock-part="totp-qr" display="flex" justifyContent="center">
        <Box
          bg="white"
          p="3"
          borderRadius="md"
          w="190px"
          h="190px"
          css={{ "& svg": { width: "100%", height: "100%", display: "block" } }}
          dangerouslySetInnerHTML={{ __html: qrSvg(uri) }}
        />
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
  );
}
