# gx stage - actions

Please add the following actions to the gx page:

- "p" => pull (use the same stashify logic as gx wt)
- "P" => push (use the same logic as gx wt and gx push with asking to force it
  if doesn't work)
- "b" => rebase on origin/master (use the same stashify logic as in gx wt, and
  do "git fetch origin" before so origin/master is up-to-date)
- "A" => commit amend - show a modal with confirmation asking if you want to
  ammend the last commit, show the commit message and a list of changes
  (filenames) from that diff (limit to 10 lines, show "..." if there are more)

NOTE:

When these commands are running I want to see their output live in an overlay popup,
if I press <ctrl-c> it should stop the process runnning inside
