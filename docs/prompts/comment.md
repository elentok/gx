# Comment

I want to map "c" in the status or commit's diff view to do the following:

1. Write the following to ~/.local/share/gx/comments/{YYYYMMDD-HHmmss-filename}.md:

```md
@path/to/file {line or line-range}

{backtick-backtick-backtick (Codeblock start)}
the diff contents
{backtick-backtick-backtick (Codeblock end)}
{blank line}
```

2. Edit this file the same way we open the commit editor (with kitty split window support)
