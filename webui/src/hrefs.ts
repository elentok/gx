export const epicHref = (address: string) => `#/e/${address}`;

export const ticketPath = (address: string) => `/t/${address}`;

export const ticketHref = (address: string) => `#${ticketPath(address)}`;

/** A Design is its map epic's canonical address (ADR 0027). */
export const designHref = (address: string) => `#/d/${address}`;

export const STATS_HREF = "#/stats";

export const AGENTS_HREF = "#/agents";

export const SETTINGS_HREF = "#/settings";

/** A session's label is its id. */
export const agentHref = (label: string) => `${AGENTS_HREF}/${label}`;
