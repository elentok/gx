import { Box, chakra, Text } from "@chakra-ui/react";
import { createContext, useContext } from "react";

export interface CertInfo {
  /** SHA-256 of the root certificate: 8 groups of 8, separated by spaces. */
  fingerprint: string;
  profile_path: string;
  cer_path: string;
}

/** `startOpen` unfolds the trust help; only the gallery sets it. */
export interface CertContextValue {
  cert?: CertInfo;
  startOpen?: boolean;
}

/** The login and setup cards sit under AuthGate, which provides the cert info the state route returned. */
export const CertContext = createContext<CertContextValue>({});

export const useCert = () => useContext(CertContext);

export const PROFILE_LABEL = "iPhone or iPad profile";
export const CER_LABEL = "Mac certificate";

export function Fingerprint({ value, size = "xs" }: { value: string; size?: "xs" | "sm" }) {
  return (
    <Box
      display="grid"
      gridTemplateColumns="repeat(4, max-content)"
      columnGap="3"
      rowGap="0.5"
      fontFamily="mono"
      fontSize={size}
      color="text.muted"
    >
      {value.split(" ").map((group, i) => (
        <span key={i}>{group}</span>
      ))}
    </Box>
  );
}

export const CertLink = ({ href, children }: { href: string; children: string }) => (
  <chakra.a href={href} color="tone.running.fg" textDecoration="underline" textUnderlineOffset="3px">
    {children}
  </chakra.a>
);
