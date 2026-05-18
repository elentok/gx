# Log filtering

I want the log view to be filterable by specific files and specific locations in those file.

Use case:

1. I'm reviewing a change in status or commit views and I want to see the history of a specific file
   or hunk/line/selection
2. I focus on the file/hunk/line/selection and press "f"
3. gx switches to the log view with filter by the current file/hunk/link/selection

In the log view:

1. I should be able to disable the filter, I'm not sure what's the best mapping for that,
   please make a few recommendations.
2. I need an indication that the log view is filtered, maybe on the frame?
   e.g. "Log (filtered by path/to/file.ts L3-L4)"
