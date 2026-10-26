# Grilling counts come from conversation records

Amends [ADR 0041](0041-stats-read-stamps-not-logs.md). The Stats page's Grilling section (words and
questions per day) reads the saved conversation records, not ticket or epic frontmatter. Grilling
belongs to a conversation, and a Design conversation can span several tickets or none. A ticket
stamp would count it on the wrong day or lose it.

Each Design record carries its counted items (message ID, time, words, questions), saved when the
conversation ends or when the server's start-up sweep recounts it. `/v1/stats` reads those records
once per request and buckets each item by its own timestamp. A message ID counts once across all
records. Stats still never read transcripts on a request: the counts were taken earlier.

Days before the earliest counted item, across all projects, are hatched as "no data". The boundary
ignores the project filter, so a project's quiet day reads as 0 and not as missing history.

The 1-second budget of ADR 0041 holds for 2,000 records (`BenchmarkStats_Grilling`). If a store
outgrows it, an in-memory cache keyed by the records dir is the next step.
