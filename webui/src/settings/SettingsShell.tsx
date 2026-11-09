import { Box, chakra, Text } from "@chakra-ui/react";
import { useState } from "react";
import { Link } from "react-router";
import type { SettingsEntry } from "../generated/server";
import { SETTINGS_HREF } from "../hrefs";
import { PageHeader } from "../page/PageHeader";
import { PageIcon } from "../page/PageIcon";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { useDevice } from "../useWide";
import { groupBySection, searchEntries } from "./settingsApi";
import { Group } from "./parts";
import { SettingsRow, type WriteSetting } from "./SettingsRow";

/** The seven global sections, in sidebar order. */
export const GLOBAL_SECTIONS = ["Execution queue", "Budget", "Notifications", "Server", "Agents", "Evolve and recovery", "Ticket store"] as const;

export const sectionSlug = (section: string) => section.toLowerCase().replace(/[^a-z0-9]+/g, "-");

export const sectionPath = (section: string) => `/settings/s/${sectionSlug(section)}`;

export const sectionFromSlug = (slug: string | undefined) => GLOBAL_SECTIONS.find((s) => sectionSlug(s) === slug);

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
  /** Startup-only keys changed since the server started. */
  pendingRestart?: string[];
  onWrite: WriteSetting;
}

function Sidebar({
  entries,
  projects,
  section,
  totpOn,
  authenticatorMissing,
  onSignOut,
  query,
  onQuery,
}: ShellProps & { query: string; onQuery: (q: string) => void }) {
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
        value={query}
        onChange={(e) => onQuery(e.target.value)}
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

export function SettingsShellView(props: ShellProps) {
  const { pendingRestart, onWrite } = props;
  const device = useDevice();
  const [query, setQuery] = useState("");
  const searching = query.trim() !== "";
  const section = props.section ?? GLOBAL_SECTIONS[0];
  const row = (e: SettingsEntry) => <SettingsRow key={e.key} entry={e} pendingRestart={pendingRestart?.includes(e.key) ?? false} onWrite={onWrite} />;
  const hits = searchEntries(props.entries, query);
  return (
    <Box display="grid" gridTemplateColumns={`${device === "desktop" ? 230 : 200}px minmax(0, 1fr)`} gap="14px" alignItems="start">
      <Sidebar {...props} section={section} query={query} onQuery={setQuery} />
      <Box data-mock-part="page" maxW="720px">
        <PageHeader title={searching ? "Search" : section} icon={<PageIcon kind="settings" />} />
        {!searching && <Group>{props.entries.filter((e) => e.section === section).map(row)}</Group>}
        {searching && hits.length === 0 && <EmptyState title="No settings match">Nothing in global settings matches “{query.trim()}”.</EmptyState>}
        {searching && hits.length > 0 && (
          <>
            <Text mt="4" fontSize="xs" letterSpacing="0.08em" textTransform="uppercase" color="text.muted" px="3">
              {hits.length} {hits.length === 1 ? "match" : "matches"} in global settings
            </Text>
            {groupBySection(hits).map((g) => (
              <Group key={g.section} label={g.section}>
                {g.entries.map(row)}
              </Group>
            ))}
          </>
        )}
      </Box>
    </Box>
  );
}
