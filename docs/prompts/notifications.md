# Notifications

I want to add an app-global notification system (to all views),
to replace the status text in the statusbar,
it should show an overlay to the top right (2 row/column margin from the edge of the screen).

There should be 5 types of notifications:

1. info - standard color
2. success - green frame and text with a ✔ icon
3. warning - orange frame and text with a  icon
4. error - red frame and text with a ✘ icon
5. in-progress/spinner - cyan frame and text with a live spinner
   - this one is for background jobs

e.g.

- "Pushed main to origin"
- "Pulled"
- "Fetching..."

- The icons should refer to the nerdfont config, if its true use a nerdfont icon, otherwise fallback
  to standard unicode icon

## Definition of done:

- No more status texts (`m.statusMsg = `)
