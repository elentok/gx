# gx pointers for mocks

Read [MOCK.md](../MOCK.md) first. This file only lists where things live in gx.

- Real components: `webui/src/ui/<name>/`, for example `webui/src/ui/button/`, `webui/src/ui/card/`, `webui/src/ui/modal/`.
- Gallery: `webui/src/gallery/registry.tsx`. The dev route is the gallery page, `gallery.html` in `webui/`.
- `PrototypeSwitcher`: `webui/src/dev/PrototypeSwitcher.tsx`. It reads `?variant=`.
- Theme file: `webui/src/ui/theme.ts`.
- Icons: Lucide through `react-icons/lu`. One Phosphor import, `PiNotePencil` from `react-icons/pi`, in `webui/src/design/Composer.tsx`. Own SVGs in `webui/src/page/PageIcon.tsx` and inline `<svg>` in other components, for example `webui/src/shell/SidebarFooter.tsx`.
- Fonts: Saira and Saira Condensed. UI library: Chakra UI v3.
- The inlined style is `<style data-mock-inlined>`. [save-route-to-html.js](save-route-to-html.js) writes it.
