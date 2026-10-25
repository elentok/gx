import { Box, chakra, Input, Textarea } from "@chakra-ui/react";
import { useEffect, useState, type FormEvent } from "react";
import { Button } from "../ui/button/Button";
import { postOpenDesign, type OpenDesign } from "./open";
import { Failure, useWrite } from "./SessionControls";

export interface NewDesignDraft {
  slug?: string;
  idea?: string;
  project?: string;
}

/** Epic name and rough idea for a Design with no map yet. The project select shows only with several projects. */
export function NewDesignForm({
  projects,
  initial = {},
  open = postOpenDesign,
  onStarted,
  onCancel,
  onChange,
}: {
  projects: string[];
  initial?: NewDesignDraft;
  open?: OpenDesign;
  /** Runs with the new Design's address once the server started it. */
  onStarted: (address: string) => void;
  onCancel: () => void;
  /** Hears every change to the fields, so the caller can keep a draft. */
  onChange?: (draft: NewDesignDraft) => void;
}) {
  const [slug, setSlug] = useState(initial.slug ?? "");
  const [idea, setIdea] = useState(initial.idea ?? "");
  const [project, setProject] = useState(initial.project ?? projects[0] ?? "");
  const { run, pending, error } = useWrite();
  useEffect(() => onChange?.({ slug, idea, project }), [onChange, slug, idea, project]);

  const submit = (e: FormEvent) => {
    e.preventDefault();
    run(
      () => open({ epic: slug.trim(), new_design: true, idea, project }),
      () => onStarted(`${project}:${slug.trim()}`),
    );
  };

  return (
    <Box as="form" aria-label="New Design" onSubmit={submit} display="flex" flexDirection="column" gap="2" mb="4" px="2">
      <Input size="sm" aria-label="Epic name" placeholder="epic-name" value={slug} onChange={(e) => setSlug(e.target.value)} />
      <Textarea size="sm" aria-label="Rough idea" placeholder="Rough idea" rows={3} value={idea} onChange={(e) => setIdea(e.target.value)} />
      {projects.length > 1 && (
        <chakra.select aria-label="Project" value={project} onChange={(e) => setProject(e.target.value)} bg="surface" borderWidth="1px" borderRadius="sm" px="2" h="26px" fontSize="sm">
          {projects.map((p) => (
            <option key={p} value={p}>
              {p}
            </option>
          ))}
        </chakra.select>
      )}
      <Box display="flex" gap="2">
        <Button type="submit" tone="running" disabled={pending}>
          Start design
        </Button>
        <Button type="button" variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
      </Box>
      <Failure>{error}</Failure>
    </Box>
  );
}
