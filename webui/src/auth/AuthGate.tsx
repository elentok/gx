import { useCallback, useEffect, useState, type ReactNode } from "react";
import { onSignedOut } from "../apiFetch";
import { Spinner } from "../ui/spinner/Spinner";
import { CertContext } from "./cert";
import { fetchAuthState, type AuthState } from "./authState";
import { LoginPage } from "./LoginPage";
import { SetupPage } from "./SetupPage";

/**
 * Shows setup, login or the app by the auth state. It never touches the hash, and the app mounts
 * only while signed in, so after login the router starts from the hash the person was on.
 */
export function AuthGate({
  children,
  fetchState = fetchAuthState,
}: {
  children: ReactNode;
  fetchState?: () => Promise<AuthState>;
}) {
  // "unknown": the read failed (network or server error). The app mounts anyway so its Offline strip shows; the connection loop does the retrying.
  const [auth, setAuth] = useState<AuthState | "unknown" | null>(null);

  const refresh = useCallback(() => {
    fetchState().then(setAuth, () => setAuth("unknown"));
  }, [fetchState]);

  useEffect(refresh, [refresh]);
  // A 401 is the signed-out signal. Re-read the state: a `gx server web-reset` leaves nothing to sign in to, so the page has to move to setup.
  useEffect(() => onSignedOut(refresh), [refresh]);

  if (!auth) return <Spinner label="Loading" />;
  // Same Provider as signed-in, so the tree keeps its shape and the app is not remounted on the transition.
  if (auth === "unknown") return <CertContext.Provider value={{}}>{children}</CertContext.Provider>;
  if (auth.state === "signed-in") return <CertContext.Provider value={{ cert: auth }}>{children}</CertContext.Provider>;
  return (
    <CertContext.Provider value={{ cert: auth }}>
      {auth.state === "setup" ? <SetupPage auth={auth} onDone={refresh} /> : <LoginPage auth={auth} onDone={refresh} />}
    </CertContext.Provider>
  );
}
