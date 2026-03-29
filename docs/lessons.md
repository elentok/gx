# Lessons

- When selecting upgrade targets for third-party dependencies, prefer the latest stable release line over release candidates or betas unless the user explicitly asks for prereleases.
- In Lip Gloss layout code, avoid subtracting border frame size twice. If a child component is already sized to the inner content area, the surrounding styled container should usually be given the full outer width and height.
- For line-level staging patches, preserve contiguous hunk structure and validate rendering fallbacks: malformed or misaligned patch/render output can silently drop delta line-number/syntax decoration and produce confusing staged diffs.
