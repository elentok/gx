# Stage - search

Please add vim-like search behavior in both the status and staged/unstaged views:

We need two modes:

- / enters search mode

- in search mode
  - while typing it should highlight matches
  - enter: goes to a "search-results" mode
  - esc: goes to normal mode

- in search-results mode
  - the search results are highlighted
  - esc: goes to normal mode (cancel highlighting) - focus should stay on the hunk/link of the result
  - n - goes to the next match
  - N/p - goes to the previous match
