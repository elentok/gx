const PATHS = {
  epic: "M3 4.5h10M3 8h10M3 11.5h6",
  design: "M10.5 2.5l3 3-7.5 7.5H3v-3l7.5-7.5z",
  epics: "M3 3.5h10M3 6.5h10M3 9.5h10M3 12.5h6",
  stats: "M3.5 13V8M8 13V3M12.5 13V6",
  cleanup: "M3.5 4.5h9M6.5 4.5V3h3v1.5M4.5 4.5l.5 8.5h6l.5-8.5M7 7v4M9 7v4",
  settings: "M3 5h10M3 11h10M6 3v4M10 9v4",
  ticket: "M3 4h10v8H3zM6 7h4M6 9.5h3",
  agent: "M3 3.5h10v7H8l-3 2.5v-2.5H3z",
} as const;

/** The large icon in a PageHeader for the pages that have a kind. */
export function PageIcon({ kind }: { kind: keyof typeof PATHS }) {
  return (
    <svg
      aria-hidden="true"
      data-page-icon={kind}
      viewBox="0 0 16 16"
      width="28"
      height="28"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d={PATHS[kind]} />
    </svg>
  );
}
