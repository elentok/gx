package ui

// ChromeRows is the app chrome above every page: the title row plus the blank
// row under it. Pages render below it, so anything a page paints at absolute
// screen cells (kitty image diffs) must add it to its row.
const ChromeRows = 2
