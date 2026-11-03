import { Box, chakra, Popover, Portal } from "@chakra-ui/react";
import { useRef, useState } from "react";

/** The project picker: a checkbox list in a popover. No ticks means "All projects". */
export function ProjectFilter({ projects, selected, onChange }: { projects: string[]; selected: string[]; onChange: (next: string[]) => void }) {
  const anchor = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);
  const label = selected.length === 0 ? "All projects" : selected.length === 1 ? selected[0] : `${selected.length} projects`;
  const toggle = (p: string) => onChange(selected.includes(p) ? selected.filter((s) => s !== p) : [...selected, p]);
  return (
    <>
      <chakra.button
        ref={anchor}
        type="button"
        aria-label="Project"
        aria-haspopup="dialog"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
        bg="surface"
        borderWidth="1px"
        borderColor="border"
        borderRadius="sm"
        px="3"
        h="40px"
        fontSize="16px"
        flexShrink="0"
        cursor="pointer"
      >
        {label} ▾
      </chakra.button>
      <Popover.Root
        open={open}
        onOpenChange={(e) => !e.open && setOpen(false)}
        finalFocusEl={() => anchor.current}
        persistentElements={[() => anchor.current]}
        positioning={{ placement: "bottom-end", gutter: 6, getAnchorRect: () => anchor.current?.getBoundingClientRect() ?? null }}
        lazyMount
        unmountOnExit
      >
        <Portal>
          <Popover.Positioner>
            <Popover.Content
              aria-label="Projects"
              bg="surface.raised"
              borderWidth="1px"
              borderColor="border"
              borderRadius="12px"
              boxShadow="lg"
              color="text"
              w="min(260px, calc(100vw - 32px))"
              py="1"
              css={{ "--popover-bg": "colors.surface.raised" }}
            >
              <chakra.button
                type="button"
                onClick={() => onChange([])}
                w="full"
                px="4"
                py="2"
                textAlign="left"
                bg="transparent"
                cursor="pointer"
                color="text.dim"
                fontSize="sm"
                disabled={selected.length === 0}
                _disabled={{ opacity: 0.5, cursor: "default" }}
              >
                All projects
              </chakra.button>
              {projects.map((p) => (
                <Box as="label" key={p} display="flex" alignItems="center" gap="3" px="4" py="2" cursor="pointer" _hover={{ bg: "tone.muted.bg" }}>
                  <input type="checkbox" checked={selected.includes(p)} onChange={() => toggle(p)} />
                  {p}
                </Box>
              ))}
            </Popover.Content>
          </Popover.Positioner>
        </Portal>
      </Popover.Root>
    </>
  );
}
