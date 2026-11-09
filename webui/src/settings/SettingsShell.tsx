import { Box, chakra, Text } from "@chakra-ui/react";
import { Link } from "react-router";
import type { SettingsEntry } from "../generated/server";
import { SETTINGS_HREF } from "../hrefs";
import { PageHeader } from "../page/PageHeader";
import { PageIcon } from "../page/PageIcon";
import { useDevice } from "../useWide";
import { groupBySection } from "./settingsApi";
import { Group } from "./parts";

/** The seven global sections, in sidebar order. */
export const GLOBAL_SECTIONS = ["Execution queue", "Budget", "Notifications", "Server", "Agents", "Evolve and recovery", "Ticket store"] as const;

export const sectionSlug = (section: string) => section.toLowerCase().replace(/[^a-z0-9]+/g, "-");

export const sectionPath = (section: string) => `/settings/s/${sectionSlug(section)}`;

export const sectionFromSlug = (slug: string | undefined) => GLOBAL_SECTIONS.find((s) => sectionSlug(s) === slug);

const humanize = (segment: string) => {
  const text = segment.replace(/-/g, " ");
  return text.charAt(0).toUpperCase() + text.slice(1);
};

const entryLabel = (e: SettingsEntry) => humanize(e.path[e.path.length - 1] ?? e.key);

function valueText(e: SettingsEntry): string {
  if (e.type === "secret") return e.set ? "Set" : "Not set";
  const v = e.set ? e.value : e.default;
  if (v === undefined || v === null || v === "") return "—";
  if (typeof v === "boolean") return v ? "on" : "off";
  if (Array.isArray(v)) return v.length ? v.join(", ") : "—";
  return typeof v === "object" ? JSON.stringify(v) : String(v);
}

const NavLink = chakra(Link);

const navLink = {
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: "1.5",
  px: "2.5",
  py: "1.5",
  borderRadius: "7px",
  color: "text.muted",
  fontWeight: "600",
  fontSize: "sm",
  cursor: "pointer",
  _hover: { bg: "surface.raised" },
} as const;

const GroupLabel = ({ children }: { children: string }) => (
  <Text fontSize="11px" letterSpacing="0.08em" textTransform="uppercase" color="text.muted" mx="2" mt="3" mb="0.5">
    {children}
  </Text>
);

const CountChip = ({ n }: { n: number }) => (
  <chakra.span fontSize="11px" fontWeight="600" px="7px" py="1px" borderRadius="full" bg="tone.running.bg" color="tone.running.fg">
    {n}
  </chakra.span>
);

export interface ShellProps {
  entries: SettingsEntry[];
  projects: string[];
  /** The section on the page; the first section when absent. */
  section?: string;
  totpOn: boolean;
  authenticatorMissing?: boolean;
  onSignOut: () => void;
}

function Sidebar({ entries, projects, section, totpOn, authenticatorMissing, onSignOut }: ShellProps) {
  const changed = new Map(groupBySection(entries).map((g) => [g.section, g.changed]));
  return (
    <chakra.nav data-mock-part="sidebar" display="grid" gap="0.5" alignSelf="start">
      <chakra.select
        data-mock-part="scope-picker"
        aria-label="Settings scope"
        w="100%"
        h="40px"
        px="3"
        borderWidth="1px"
        borderColor="border"
        borderRadius="8px"
        bg="surface.raised"
        color="text"
        defaultValue="global"
      >
        <option value="global">Global settings</option>
        {projects.map((p) => (
          <option key={p} value={p}>
            Project: {p}
          </option>
        ))}
      </chakra.select>
      <chakra.input
        data-mock-part="search"
        type="text"
        aria-label="Search settings"
        placeholder="Search all settings"
        mt="2"
        w="100%"
        h="40px"
        px="3.5"
        borderWidth="1px"
        borderColor="border"
        borderRadius="10px"
        bg="surface.raised"
        color="text"
      />
      <GroupLabel>Global</GroupLabel>
      {GLOBAL_SECTIONS.map((s) => (
        <NavLink
          key={s}
          to={sectionPath(s)}
          {...navLink}
          aria-current={s === section ? "page" : undefined}
          {...(s === section && { bg: "tone.running.bg", color: "tone.running.fg" })}
        >
          {s}
          {(changed.get(s) ?? 0) > 0 && <CountChip n={changed.get(s) ?? 0} />}
        </NavLink>
      ))}
      <GroupLabel>Account</GroupLabel>
      <AccountLink href={`${SETTINGS_HREF}/logins`}>Web logins</AccountLink>
      <AccountLink href={`${SETTINGS_HREF}/password`}>Password</AccountLink>
      {totpOn && <AccountLink href={`${SETTINGS_HREF}/authenticator`}>{authenticatorMissing ? "Add authenticator" : "Authenticator"}</AccountLink>}
      <AccountLink href={`${SETTINGS_HREF}/certificate`}>Certificate</AccountLink>
      <chakra.button type="button" onClick={onSignOut} {...navLink} color="tone.deadlocked.fg" textAlign="left">
        Sign out of this device
      </chakra.button>
    </chakra.nav>
  );
}

const AccountLink = ({ href, children }: { href: string; children: string }) => (
  <chakra.a href={href} {...navLink}>
    {children}
    <Text aria-hidden color="text.dim">
      ›
    </Text>
  </chakra.a>
);

function ReadOnlyRow({ entry }: { entry: SettingsEntry }) {
  return (
    <Box px="3.5" py="3" display="grid" gap="1.5" _notFirst={{ borderTopWidth: "1px", borderTopColor: "hairline" }}>
      <Box display="flex" gap="2" alignItems="center" flexWrap="wrap">
        <Text fontWeight="600">{entryLabel(entry)}</Text>
        <Box flex="1" />
        <Text fontSize="sm" color="text.muted" data-testid="setting-value">
          {valueText(entry)}
        </Text>
      </Box>
      <Text fontSize="13px" color="text.muted" maxW="62ch">
        {entry.description}
      </Text>
      <Text fontFamily="mono" fontSize="11px" color="text.dim">
        {entry.key}
      </Text>
    </Box>
  );
}

export function SettingsShellView(props: ShellProps) {
  const device = useDevice();
  const section = props.section ?? GLOBAL_SECTIONS[0];
  const rows = props.entries.filter((e) => e.section === section);
  return (
    <Box display="grid" gridTemplateColumns={`${device === "desktop" ? 230 : 200}px minmax(0, 1fr)`} gap="14px" alignItems="start">
      <Sidebar {...props} section={section} />
      <Box data-mock-part="page" maxW="720px">
        <PageHeader title={section} icon={<PageIcon kind="settings" />} />
        <Group>
          {rows.map((e) => (
            <ReadOnlyRow key={e.key} entry={e} />
          ))}
        </Group>
      </Box>
    </Box>
  );
}
