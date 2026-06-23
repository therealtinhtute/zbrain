// Conflict detection on note write.
// If a new note targets a path that an `active` note already covers and no
// supersede is declared, refuse the write. See SPEC AC-P1-3.

import { existsSync } from "node:fs";
import { join } from "node:path";
import { notePath, type Note, readNote } from "./note-service";
import type { RuntimePaths } from "./runtime-paths";
import { isWikiTier, type WikiTier } from "./workspace-layout";

export interface ConflictReport {
  reason: "path-overlap";
  existing: Note;
  proposedPath: string;
  proposedTier: WikiTier;
  proposedSlug: string;
}

export function detectConflict(
  paths: RuntimePaths,
  workspace: string,
  tier: WikiTier,
  slug: string,
  declaringSupersedes: string[] = [],
): ConflictReport | null {
  if (!isWikiTier(tier)) {
    return null;
  }
  const proposedPath = notePath(paths, workspace, tier, slug);
  if (!existsSync(proposedPath)) {
    return null;
  }
  const existing = readNote(paths, workspace, tier, slug);
  if (!existing) {
    return null;
  }
  if (existing.status !== "active") {
    return null;
  }
  if (declaringSupersedes.length > 0) {
    return null;
  }
  return {
    reason: "path-overlap",
    existing,
    proposedPath: join(tier, `${slug}.md`),
    proposedTier: tier,
    proposedSlug: slug,
  };
}
