import { LoginAuthenticatorCard, LoginCard } from "../auth/LoginPage";
import { PASSWORD_TOO_SHORT, rateLimitText, reasonText, TOTP_MISMATCH_TEXT as TOTP_MISMATCH, WRONG_CREDENTIALS } from "../auth/refusals";
import { AuthenticatorView, ChangePasswordView } from "../settings/AccountPages";
import { CertificateView, SettingsHomeView, WebLoginsView, WebLoginView } from "../settings/SettingsPage";
import { CertContext } from "../auth/cert";
import { SetupCodeCard, SetupPasswordCard, SetupTotpCard } from "../auth/SetupPage";
import type { GalleryEntry } from "./registry";

const GALLERY_CERT = {
  fingerprint: "3F9A1C7E 52B0D488 0E61A73B C912F54D 6AE09927 B35C0871 DE441F8B A263C05E",
  profile_path: "/gx-root.mobileconfig",
  cer_path: "/gx-root.cer",
};
const GALLERY_OTPAUTH_URI ="otpauth://totp/gx?secret=JBSWY3DPEHPK3PXP4N2Q7ZLA&issuer=gx";

export const authEntries: GalleryEntry[] = [
  {
    name: "LoginCard",
    cases: [
      { label: "default", node: <LoginCard totpOn /> },
      { label: "wrong password or code", node: <LoginCard totpOn password="hunter2hunter2" error={reasonText({ reason: WRONG_CREDENTIALS })} /> },
      { label: "TOTP off", node: <LoginCard totpOn={false} /> },
      { label: "setup code", node: <LoginCard totpOn setupCodeRequired setupCode="K7QM-3XPD" /> },
      {
        label: "setup code wrong or expired",
        node: (
          <LoginCard
            totpOn
            setupCodeRequired
            setupCode="K7QM-3XPP"
            error="That code is wrong or expired. Run gx server status for a new one."
          />
        ),
      },
      {
        label: "add authenticator",
        node: <LoginAuthenticatorCard secret="JBSWY3DPEHPK3PXP4N2Q7ZLA" uri={GALLERY_OTPAUTH_URI} />,
      },
      {
        label: "add authenticator, clock mismatch",
        node: <LoginAuthenticatorCard secret="JBSWY3DPEHPK3PXP4N2Q7ZLA" uri={GALLERY_OTPAUTH_URI} error={TOTP_MISMATCH} />,
      },
      {
        label: "rate limited",
        node: <LoginCard totpOn password="hunter2hunter2" error={rateLimitText(181)} submitDisabled />,
      },
      {
        label: "certificate help open",
        node: (
          <CertContext.Provider value={{ cert: GALLERY_CERT, startOpen: true }}>
            <LoginCard totpOn />
          </CertContext.Provider>
        ),
      },
    ],
  },
  {
    name: "SettingsHome",
    cases: [
      { label: "default", node: <SettingsHomeView loginCount={4} totpOn onSignOut={() => {}} /> },
      { label: "with Agents row", node: <SettingsHomeView loginCount={4} totpOn defaultHarness="codex" onSignOut={() => {}} /> },
      { label: "TOTP off", node: <SettingsHomeView loginCount={1} totpOn={false} onSignOut={() => {}} /> },
      { label: "no saved authenticator", node: <SettingsHomeView loginCount={1} totpOn authenticatorMissing onSignOut={() => {}} /> },
    ],
  },
  {
    name: "WebLoginsView",
    cases: [
      {
        label: "default",
        node: (
          <WebLoginsView
            now={Date.now()}
            logins={[
              { id: "a", name: "Safari on iPhone", last_used: new Date().toISOString(), last_ip: "192.168.1.23", this_device: true },
              { id: "b", name: "Chrome on Mac", last_used: new Date(Date.now() - 2 * 3600_000).toISOString(), last_ip: "100.101.12.4", this_device: false },
              { id: "c", name: "Firefox on Linux", last_used: new Date(Date.now() - 71 * 86400_000).toISOString(), last_ip: "192.168.1.40", this_device: false },
            ]}
          />
        ),
      },
      {
        label: "desktop, four logins",
        node: (
          <WebLoginsView
            now={Date.now()}
            logins={[
              { id: "a", name: "Safari on iPhone", last_used: new Date().toISOString(), last_ip: "192.168.1.23", this_device: true },
              { id: "b", name: "Chrome on Mac", last_used: new Date(Date.now() - 2 * 3600_000).toISOString(), last_ip: "100.101.12.4", this_device: false },
              { id: "c", name: "Home-screen app on iPad", last_used: new Date(Date.now() - 12 * 86400_000).toISOString(), last_ip: "192.168.1.40", this_device: false },
              { id: "d", name: "Firefox on Linux", last_used: new Date(Date.now() - 71 * 86400_000).toISOString(), last_ip: "192.168.1.41", this_device: false },
            ]}
          />
        ),
      },
    ],
  },
  {
    name: "WebLoginView",
    cases: [
      {
        label: "another device",
        node: (
          <WebLoginView
            now={Date.now()}
            login={{
              id: "d",
              name: "Firefox on Linux",
              created: new Date(Date.now() - 80 * 86400_000).toISOString(),
              last_used: new Date(Date.now() - 71 * 86400_000).toISOString(),
              last_ip: "100.77.3.9",
              this_device: false,
            }}
            onRename={() => {}}
            onSignOut={() => {}}
          />
        ),
      },
      {
        label: "confirm sign out",
        node: (
          <WebLoginView
            startConfirming
            now={Date.now()}
            login={{
              id: "d",
              name: "Firefox on Linux",
              created: new Date(Date.now() - 80 * 86400_000).toISOString(),
              last_used: new Date(Date.now() - 71 * 86400_000).toISOString(),
              last_ip: "100.77.3.9",
              this_device: false,
            }}
            onRename={() => {}}
            onSignOut={() => {}}
          />
        ),
      },
      {
        label: "this device",
        node: (
          <WebLoginView
            now={Date.now()}
            login={{
              id: "a",
              name: "Safari on iPhone",
              created: new Date(Date.now() - 20 * 86400_000).toISOString(),
              last_used: new Date().toISOString(),
              last_ip: "192.168.1.23",
              this_device: true,
            }}
            onRename={() => {}}
            onSignOut={() => {}}
          />
        ),
      },
    ],
  },
  {
    name: "CertificateView",
    cases: [{ label: "default", node: <CertificateView cert={GALLERY_CERT} /> }],
  },
  {
    name: "ChangePasswordView",
    cases: [
      { label: "default", node: <ChangePasswordView otherDevices={3} totpOn onSubmit={() => {}} /> },
      { label: "TOTP off", node: <ChangePasswordView otherDevices={3} totpOn={false} onSubmit={() => {}} /> },
      {
        label: "too short",
        node: <ChangePasswordView otherDevices={1} totpOn onSubmit={() => {}} error={reasonText({ reason: PASSWORD_TOO_SHORT })} />,
      },
    ],
  },
  {
    name: "AuthenticatorView",
    cases: [
      { label: "replace, step 1", node: <AuthenticatorView mode="replace"otherDevices={3} onBegin={() => {}} onFinish={() => {}} /> },
      {
        label: "replace, step 2",
        node: (
          <AuthenticatorView
            mode="replace"
            otherDevices={3}
            next={{ secret: "JBSWY3DPEHPK3PXP4N2Q7ZLA", uri: "otpauth://totp/gx?secret=JBSWY3DPEHPK3PXP4N2Q7ZLA" }}
            onBegin={() => {}}
            onFinish={() => {}}
          />
        ),
      },
      { label: "add authenticator", node: <AuthenticatorView mode="add" otherDevices={3} onBegin={() => {}} onFinish={() => {}} /> },
    ],
  },
  {
    name: "SetupCodeCard",
    cases: [
      { label: "default", node: <SetupCodeCard totpOn code="K7QM-3XPD" /> },
      { label: "TOTP off", node: <SetupCodeCard totpOn={false} code="K7QM-3XPD" /> },
      {
        label: "wrong or expired",
        node: (
          <SetupCodeCard
            totpOn
            code="K7QM-3XPP"
            error="That code is wrong or expired. Run gx server status for a new one."
          />
        ),
      },
    ],
  },
  {
    name: "SetupPasswordCard",
    cases: [
      { label: "default", node: <SetupPasswordCard totpOn /> },
      { label: "TOTP off", node: <SetupPasswordCard totpOn={false} /> },
    ],
  },
  {
    name: "SetupTotpCard",
    cases: [
      { label: "default", node: <SetupTotpCard secret="JBSWY3DPEHPK3PXP4N2Q7ZLA" uri={GALLERY_OTPAUTH_URI} /> },
      {
        label: "wrong code",
        node: <SetupTotpCard secret="JBSWY3DPEHPK3PXP4N2Q7ZLA" uri={GALLERY_OTPAUTH_URI} error={TOTP_MISMATCH} />,
      },
    ],
  },
];
