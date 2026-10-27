import { Text } from "@chakra-ui/react";
import { Link } from "react-router";
import { useWide } from "../useWide";

/** `waiting` is the number of other epics in Needs you; it reads inside the link and is left out at zero. */
export function BackLink({ to, label, waiting = 0 }: { to: string; label: string; waiting?: number }) {
  const wide = useWide();
  if (waiting <= 0) return <Link to={to}>{label}</Link>;
  const full = `${waiting} ${waiting === 1 ? "more needs you" : "more need you"}`;
  return (
    <Link to={to} aria-label={`${label} · ${full}`} style={{ whiteSpace: "nowrap" }}>
      {label}
      <Text as="span" aria-hidden ml="2" fontSize="13px" color="tone.waiting.fg">
        · {wide ? full : `${waiting} more`}
      </Text>
    </Link>
  );
}
