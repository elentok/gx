import type { CertInfo } from "./cert";

/** The certificate fields every auth state carries, for the tests that build one. */
export const TEST_CERT: CertInfo = {
  fingerprint: "AAAA1111 BBBB2222 CCCC3333 DDDD4444 EEEE5555 FFFF6666 00007777 88889999",
  profile_path: "/gx-root.mobileconfig",
  cer_path: "/gx-root.cer",
};
